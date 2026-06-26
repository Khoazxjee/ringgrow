package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"time"
)

const (
	maxImageBytes = 50 << 20
	defaultPrompt = `Edit the provided product photo of rings for premium jewelry ecommerce with a very light, conservative retouch. Preserve the exact ring geometry, camera angle, crop, composition, scale, engravings, gemstone count and placement, decorative rectangular panels, edges, inner contours, shadows, surface texture, and all ornamental details. Do not enhance, redraw, simplify, sharpen, clean up, or reinterpret the patterns and small details. Only make a subtle color and lighting adjustment. Target a clean studio jewelry gold tone: bright natural 18K yellow gold with pale champagne highlights, medium honey-gold midtones, and gentle amber reflections inside the rims and along polished edges. The color should feel like a real product photo under softbox studio lighting, not a dark orange or saturated yellow filter. Keep contrast polished but smooth, with realistic metal reflections and a white or neutral studio background. Do not add, remove, reshape, resize, repaint, blur, simplify, or invent any ring details.`
)

type app struct {
	client  *http.Client
	apiKey  string
	baseURL string
	model   string
	compat  bool
}

type enhanceResponse struct {
	Image        string         `json:"image"`
	MimeType     string         `json:"mimeType"`
	Model        string         `json:"model"`
	Prompt       string         `json:"prompt"`
	Quality      string         `json:"quality,omitempty"`
	Size         string         `json:"size,omitempty"`
	OutputFormat string         `json:"outputFormat,omitempty"`
	Usage        map[string]any `json:"usage,omitempty"`
}

type openAIImagesResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
	OutputFormat string         `json:"output_format"`
	Quality      string         `json:"quality"`
	Size         string         `json:"size"`
	Usage        map[string]any `json:"usage"`
	Error        *openAIError   `json:"error"`
}

type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code"`
}

func main() {
	a := &app{
		client: &http.Client{
			Timeout: 4 * time.Minute,
		},
		apiKey:  os.Getenv("OPENAI_API_KEY"),
		baseURL: strings.TrimRight(env("OPENAI_BASE_URL", "https://api.openai.com/v1"), "/"),
		model:   env("OPENAI_IMAGE_MODEL", "gpt-image-2"),
	}
	a.compat = !strings.Contains(a.baseURL, "api.openai.com")

	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.health)
	mux.HandleFunc("/api/enhance", a.enhance)

	addr := ":" + env("PORT", "8080")
	log.Printf("RingGlow backend listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
		log.Fatal(err)
	}
}

func (a *app) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"model": a.model,
	})
}

func (a *app) enhance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if a.apiKey == "" {
		writeError(w, http.StatusInternalServerError, "OPENAI_API_KEY is not configured")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxImageBytes+1<<20)
	if err := r.ParseMultipartForm(maxImageBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form or image is too large")
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "image file is required")
		return
	}
	defer file.Close()

	if header.Size <= 0 || header.Size > maxImageBytes {
		writeError(w, http.StatusBadRequest, "image must be less than 50MB")
		return
	}

	contentType := header.Header.Get("Content-Type")
	if !isSupportedImage(contentType, header.Filename) {
		writeError(w, http.StatusBadRequest, "image must be jpg, png, or webp")
		return
	}

	quality := pick(r.FormValue("quality"), "high", []string{"auto", "high", "medium", "low"})
	size := pick(r.FormValue("size"), "auto", []string{"auto", "1024x1024", "1536x1024", "1024x1536"})
	outputFormat := pick(r.FormValue("output_format"), "png", []string{"png", "jpeg", "webp"})

	result, effectiveSize, err := a.editImage(file, header, quality, size, outputFormat)
	if err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			writeError(w, apiErr.status, apiErr.message)
			return
		}
		log.Printf("enhance failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to enhance image")
		return
	}

	format := result.OutputFormat
	if format == "" {
		format = outputFormat
	}
	mimeType := mimeForFormat(format)
	image := result.Data[0].URL
	if result.Data[0].B64JSON != "" {
		image = "data:" + mimeType + ";base64," + result.Data[0].B64JSON
	}

	writeJSON(w, http.StatusOK, enhanceResponse{
		Image:        image,
		MimeType:     mimeType,
		Model:        a.model,
		Prompt:       defaultPrompt,
		Quality:      firstNonEmpty(result.Quality, quality),
		Size:         firstNonEmpty(result.Size, effectiveSize),
		OutputFormat: format,
		Usage:        result.Usage,
	})
}

func (a *app) editImage(file multipart.File, header *multipart.FileHeader, quality, size, outputFormat string) (*openAIImagesResponse, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="%s"`, escapeQuotes(header.Filename)))
	partHeader.Set("Content-Type", imageContentType(header))

	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return nil, "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, "", err
	}

	effectiveSize := size
	if a.compat && effectiveSize == "auto" {
		effectiveSize = "1024x1024"
	}

	fields := map[string]string{
		"model":  a.model,
		"prompt": defaultPrompt,
		"size":   effectiveSize,
	}
	if !a.compat {
		fields["n"] = "1"
		fields["quality"] = quality
		fields["output_format"] = outputFormat
		fields["background"] = "auto"
		fields["input_fidelity"] = "high"
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}

	req, err := http.NewRequest(http.MethodPost, a.baseURL+"/images/edits", &body)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	var parsed openAIImagesResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return nil, "", fmt.Errorf("decode OpenAI response: %w; body: %s", err, responsePreview(responseBody))
	}

	if parsed.Error != nil {
		return nil, "", &apiError{status: http.StatusBadGateway, message: parsed.Error.Message}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := "OpenAI image edit failed"
		if parsed.Error != nil && parsed.Error.Message != "" {
			msg = parsed.Error.Message
		}
		return nil, "", &apiError{status: http.StatusBadGateway, message: msg}
	}

	if len(parsed.Data) == 0 || (parsed.Data[0].B64JSON == "" && parsed.Data[0].URL == "") {
		return nil, "", fmt.Errorf("OpenAI response did not include image data; body: %s", responsePreview(responseBody))
	}

	return &parsed, effectiveSize, nil
}

type apiError struct {
	status  int
	message string
}

func (e *apiError) Error() string {
	return e.message
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isSupportedImage(contentType, filename string) bool {
	contentType = strings.ToLower(contentType)
	if contentType == "image/jpeg" || contentType == "image/png" || contentType == "image/webp" {
		return true
	}

	name := strings.ToLower(filename)
	return strings.HasSuffix(name, ".jpg") ||
		strings.HasSuffix(name, ".jpeg") ||
		strings.HasSuffix(name, ".png") ||
		strings.HasSuffix(name, ".webp")
}

func imageContentType(header *multipart.FileHeader) string {
	contentType := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if contentType == "image/jpeg" || contentType == "image/png" || contentType == "image/webp" {
		return contentType
	}

	name := strings.ToLower(header.Filename)
	switch {
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".webp"):
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

func escapeQuotes(value string) string {
	return strings.ReplaceAll(value, `"`, `\"`)
}

func pick(value, fallback string, allowed []string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, item := range allowed {
		if value == item {
			return item
		}
	}
	return fallback
}

func mimeForFormat(format string) string {
	switch format {
	case "jpeg", "jpg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func responsePreview(body []byte) string {
	const maxPreviewBytes = 800
	text := strings.TrimSpace(string(body))
	if len(text) > maxPreviewBytes {
		return text[:maxPreviewBytes] + "...(truncated)"
	}
	return text
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response failed: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
