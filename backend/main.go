package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxImageBytes        = 50 << 20
	maxBatchArchiveBytes = 500 << 20
	defaultPrompt        = `Edit the first input image as the only subject photo for premium jewelry ecommerce. If a second input image is present, use it only as the fixed color, exposure, contrast, and studio-lighting calibration reference, never as a shape, composition, crop, gemstone, pattern, or object reference.

Reference matching is the main requirement: every output must follow the reference image's realistic studio lighting, but the metal color should be a richer, more vivid 18K yellow gold than a pale champagne baseline. Normalize the metal to the same rich polished yellow-gold studio palette across all inputs, regardless of whether the source is pale, dark, orange, reddish, oversaturated, underexposed, or already gold. Two photos of the same ring with different original colors must come out with the same perceived gold brightness, hue, saturation, and gloss. Make the gold visibly deeper and more saturated so it pops as real gold, while keeping clean bright champagne highlights, natural yellow-gold midtones, controlled honey reflections inside polished rims, and realistic softbox shine. Do not make it orange, copper, brown, neon yellow, or over-contrasted. If the source image is dark or very warm, lift and cool it back to this rich studio gold; if the source is pale, enrich it to this stronger yellow-gold standard. Keep a white or neutral studio background with soft product-photo shadows. The result should look like the same studio lighting setup and the same gold color standard across hundreds of different inputs.

Preserve the first image's exact ring geometry, camera angle, crop, composition, scale, engravings, gemstone count and placement, decorative rectangular panels, edges, inner contours, shadows, surface texture, and all ornamental details. Make the visible metal edges and panel borders clean in a realistic studio product-photo way: reduce muddy halos, color fringing, compression artifacts, and rough lighting transitions along silhouettes and polished edges while keeping natural micro-reflections. Clean should mean true photographic clarity, not airbrushed perfection. Do not over-sharpen, redraw, simplify, reinterpret, add, remove, reshape, resize, repaint, blur, invent details, make mathematically perfect outlines, or make the rings look CGI, plastic, chrome, copper, bronze, or overly retouched. Only recolor and relight the metal to the fixed natural studio gold standard with real clean edges. Avoid dark orange, amber-heavy shadows, heavy yellow filters, fake saturated gold, burnt highlights, and excessive contrast.`
)

type app struct {
	client               *http.Client
	apiKey               string
	driveAPIKey          string
	baseURL              string
	model                string
	compat               bool
	referenceImage       []byte
	referenceFilename    string
	referenceContentType string
	referencePalette     goldPalette
}

type goldPalette struct {
	Hue        float64
	Saturation float64
	Lightness  float64
	Ready      bool
}

type goldImageStats struct {
	Count      int
	Hue        float64
	Saturation float64
	Lightness  float64
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

type batchRequest struct {
	Source    string `json:"source"`
	OutputDir string `json:"outputDir"`
	Quality   string `json:"quality,omitempty"`
	Size      string `json:"size,omitempty"`
}

type batchResponse struct {
	Total     int               `json:"total"`
	Succeeded int               `json:"succeeded"`
	Failed    int               `json:"failed"`
	OutputDir string            `json:"outputDir"`
	Items     []batchResultItem `json:"items"`
}

type batchResultItem struct {
	Name   string `json:"name"`
	Input  string `json:"input"`
	Output string `json:"output,omitempty"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type batchInputFile struct {
	Path        string
	Name        string
	ContentType string
}

type driveFileItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MimeType    string `json:"mimeType"`
	Size        string `json:"size,omitempty"`
	ResourceKey string `json:"resourceKey,omitempty"`
}

type driveFilesListResponse struct {
	Files         []driveFileItem `json:"files"`
	NextPageToken string          `json:"nextPageToken"`
	Error         *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
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
	referenceImage, referenceFilename, referenceContentType := loadGoldReference()
	referencePalette := analyzeGoldPalette(referenceImage)
	a := &app{
		client: &http.Client{
			Timeout: 4 * time.Minute,
		},
		apiKey:               os.Getenv("OPENAI_API_KEY"),
		driveAPIKey:          strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_API_KEY")),
		baseURL:              strings.TrimRight(env("OPENAI_BASE_URL", "https://api.openai.com/v1"), "/"),
		model:                env("OPENAI_IMAGE_MODEL", "gpt-image-2"),
		referenceImage:       referenceImage,
		referenceFilename:    referenceFilename,
		referenceContentType: referenceContentType,
		referencePalette:     referencePalette,
	}
	a.compat = !strings.Contains(a.baseURL, "api.openai.com")
	if len(a.referenceImage) > 0 {
		log.Printf("Using gold color reference image: %s", a.referenceFilename)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.health)
	mux.HandleFunc("/api/enhance", a.enhance)
	mux.HandleFunc("/api/batch", a.batch)

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

	imageBytes, mimeType, err := a.resultImageBytes(result)
	if err != nil {
		log.Printf("read enhanced image failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to read enhanced image")
		return
	}
	imageBytes, mimeType, err = a.finalizeImageBytes(imageBytes, mimeType, format)
	if err != nil {
		log.Printf("finalize enhanced image failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to finalize enhanced image")
		return
	}
	image := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(imageBytes)

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

func (a *app) batch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if a.apiKey == "" {
		writeError(w, http.StatusInternalServerError, "OPENAI_API_KEY is not configured")
		return
	}

	var request batchRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid batch request")
		return
	}

	request.Source = strings.TrimSpace(request.Source)
	request.OutputDir = strings.TrimSpace(request.OutputDir)
	if request.Source == "" {
		writeError(w, http.StatusBadRequest, "source folder or URL is required")
		return
	}
	if request.OutputDir == "" {
		writeError(w, http.StatusBadRequest, "output folder is required")
		return
	}

	quality := pick(request.Quality, "high", []string{"auto", "high", "medium", "low"})
	size := pick(request.Size, "auto", []string{"auto", "1024x1024", "1536x1024", "1024x1536"})

	outputDir, err := filepath.Abs(request.OutputDir)
	if err != nil {
		writeError(w, http.StatusBadRequest, "output folder path is invalid")
		return
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		writeError(w, http.StatusBadRequest, "cannot create output folder")
		return
	}

	inputs, cleanup, err := a.resolveBatchInputs(request.Source)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(inputs) == 0 {
		writeError(w, http.StatusBadRequest, "no jpg, png, or webp images found")
		return
	}

	response := batchResponse{
		Total:     len(inputs),
		OutputDir: outputDir,
		Items:     make([]batchResultItem, 0, len(inputs)),
	}
	for _, input := range inputs {
		item := batchResultItem{
			Name:   input.Name,
			Input:  input.Path,
			Status: "failed",
		}

		outputPath := filepath.Join(outputDir, filepath.Base(input.Name))
		if err := a.processBatchImage(input, outputPath, quality, size); err != nil {
			item.Error = err.Error()
			response.Failed++
		} else {
			item.Output = outputPath
			item.Status = "done"
			response.Succeeded++
		}
		response.Items = append(response.Items, item)
	}

	writeJSON(w, http.StatusOK, response)
}

func (a *app) editImage(file multipart.File, header *multipart.FileHeader, quality, size, outputFormat string) (*openAIImagesResponse, string, error) {
	sourceImage, err := io.ReadAll(file)
	if err != nil {
		return nil, "", err
	}
	return a.editImageBytes(sourceImage, header.Filename, imageContentType(header), quality, size, outputFormat)
}

func (a *app) editImageBytes(sourceImage []byte, filename, contentType, quality, size, outputFormat string) (*openAIImagesResponse, string, error) {
	if len(sourceImage) == 0 || len(sourceImage) > maxImageBytes {
		return nil, "", &apiError{status: http.StatusBadRequest, message: "image must be less than 50MB"}
	}

	effectiveSize := size
	if a.compat && effectiveSize == "auto" {
		effectiveSize = "1024x1024"
	}

	if len(a.referenceImage) > 0 {
		result, err := a.sendEditRequest(sourceImage, filename, contentType, quality, effectiveSize, outputFormat, true)
		if err == nil {
			return result, effectiveSize, nil
		}
		if shouldRetryWithoutReference(err) {
			log.Printf("gold reference request failed, retrying without reference image: %v", err)
		} else {
			return nil, "", err
		}
	}

	result, err := a.sendEditRequest(sourceImage, filename, contentType, quality, effectiveSize, outputFormat, false)
	return result, effectiveSize, err
}

func (a *app) sendEditRequest(sourceImage []byte, filename, contentType, quality, size, outputFormat string, includeReference bool) (*openAIImagesResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	imageFieldName := "image"
	if includeReference {
		imageFieldName = "image[]"
	}
	if err := writeImagePart(writer, imageFieldName, filename, contentType, sourceImage); err != nil {
		return nil, err
	}
	if includeReference {
		if err := writeImagePart(writer, imageFieldName, a.referenceFilename, a.referenceContentType, a.referenceImage); err != nil {
			return nil, err
		}
	}

	fields := map[string]string{
		"model":  a.model,
		"prompt": defaultPrompt,
		"size":   size,
	}
	if !a.compat {
		fields["n"] = "1"
		fields["quality"] = quality
		fields["output_format"] = outputFormat
		fields["background"] = "auto"
		if a.model != "gpt-image-2" {
			fields["input_fidelity"] = "high"
		}
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, a.baseURL+"/images/edits", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed openAIImagesResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode OpenAI response: %w; body: %s", err, responsePreview(responseBody))
	}

	if parsed.Error != nil {
		return nil, &apiError{status: http.StatusBadGateway, providerStatus: resp.StatusCode, message: parsed.Error.Message}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := "OpenAI image edit failed"
		if parsed.Error != nil && parsed.Error.Message != "" {
			msg = parsed.Error.Message
		}
		return nil, &apiError{status: http.StatusBadGateway, providerStatus: resp.StatusCode, message: msg}
	}

	if len(parsed.Data) == 0 || (parsed.Data[0].B64JSON == "" && parsed.Data[0].URL == "") {
		return nil, fmt.Errorf("OpenAI response did not include image data; body: %s", responsePreview(responseBody))
	}

	return &parsed, nil
}

func (a *app) processBatchImage(input batchInputFile, outputPath, quality, size string) error {
	sourceImage, err := os.ReadFile(input.Path)
	if err != nil {
		return fmt.Errorf("không đọc được ảnh gốc")
	}

	outputFormat := outputFormatForFilename(input.Name)
	if outputFormat == "" {
		return fmt.Errorf("định dạng ảnh không được hỗ trợ")
	}

	result, _, err := a.editImageBytes(sourceImage, input.Name, input.ContentType, quality, size, outputFormat)
	if err != nil {
		return err
	}

	imageBytes, mimeType, err := a.resultImageBytes(result)
	if err != nil {
		return err
	}

	outputBytes, _, err := a.finalizeImageBytes(imageBytes, mimeType, outputFormat)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outputPath, outputBytes, 0644); err != nil {
		return fmt.Errorf("không lưu được ảnh kết quả")
	}
	return nil
}

func (a *app) resultImageBytes(result *openAIImagesResponse) ([]byte, string, error) {
	if len(result.Data) == 0 {
		return nil, "", fmt.Errorf("API không trả ảnh kết quả")
	}

	mimeType := mimeForFormat(result.OutputFormat)
	if result.Data[0].B64JSON != "" {
		data, err := base64.StdEncoding.DecodeString(result.Data[0].B64JSON)
		if err != nil {
			return nil, "", fmt.Errorf("không đọc được ảnh base64 từ API")
		}
		if detected := detectImageMime(data); detected != "" {
			mimeType = detected
		}
		return data, mimeType, nil
	}

	if result.Data[0].URL == "" {
		return nil, "", fmt.Errorf("API không trả dữ liệu ảnh")
	}

	data, _, contentType, err := a.downloadRemoteFile(result.Data[0].URL)
	if err != nil {
		return nil, "", err
	}
	if detected := detectImageMime(data); detected != "" {
		contentType = detected
	}
	return data, contentType, nil
}

func (a *app) resolveBatchInputs(source string) ([]batchInputFile, func(), error) {
	if isHTTPURL(source) {
		return a.resolveRemoteBatchInputs(source)
	}

	info, err := os.Stat(source)
	if err != nil {
		return nil, nil, fmt.Errorf("không tìm thấy folder hoặc file nguồn")
	}

	if !info.IsDir() {
		if !isSupportedImage("", source) {
			return nil, nil, fmt.Errorf("file nguồn phải là JPG, PNG hoặc WEBP")
		}
		abs, err := filepath.Abs(source)
		if err != nil {
			return nil, nil, fmt.Errorf("đường dẫn file nguồn không hợp lệ")
		}
		return []batchInputFile{{
			Path:        abs,
			Name:        filepath.Base(source),
			ContentType: contentTypeForFilename(source),
		}}, nil, nil
	}

	entries, err := os.ReadDir(source)
	if err != nil {
		return nil, nil, fmt.Errorf("không đọc được folder nguồn")
	}

	inputs := make([]batchInputFile, 0)
	for _, entry := range entries {
		if entry.IsDir() || !isSupportedImage("", entry.Name()) {
			continue
		}
		path := filepath.Join(source, entry.Name())
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, nil, fmt.Errorf("đường dẫn ảnh nguồn không hợp lệ")
		}
		inputs = append(inputs, batchInputFile{
			Path:        abs,
			Name:        entry.Name(),
			ContentType: contentTypeForFilename(entry.Name()),
		})
	}

	sort.Slice(inputs, func(i, j int) bool {
		return strings.ToLower(inputs[i].Name) < strings.ToLower(inputs[j].Name)
	})
	return inputs, nil, nil
}

func (a *app) resolveRemoteBatchInputs(source string) ([]batchInputFile, func(), error) {
	tmpDir, err := os.MkdirTemp("", "ringglow-batch-*")
	if err != nil {
		return nil, nil, fmt.Errorf("không tạo được folder tạm")
	}
	cleanup := func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			log.Printf("cleanup batch temp failed: %v", err)
		}
	}

	if folderID, resourceKey, isDrive, isFolder := driveFileID(source); isDrive && isFolder {
		inputs, err := a.resolveDriveFolderInputs(folderID, resourceKey, tmpDir)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		return inputs, cleanup, nil
	}

	data, filename, contentType, err := a.downloadRemoteBatchSource(source)
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	switch {
	case isZipArchive(data, filename, contentType):
		inputs, err := extractBatchArchive(data, tmpDir)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		return inputs, cleanup, nil
	case isSupportedImage(contentType, filename):
		name := fallbackRemoteFilename(filename, contentType)
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, data, 0644); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("không lưu được ảnh tải về")
		}
		return []batchInputFile{{
			Path:        path,
			Name:        name,
			ContentType: contentTypeForFilename(name),
		}}, cleanup, nil
	default:
		cleanup()
		return nil, nil, fmt.Errorf("link phải trỏ tới ảnh JPG/PNG/WEBP hoặc file ZIP công khai")
	}
}

func (a *app) downloadRemoteBatchSource(source string) ([]byte, string, string, error) {
	candidates, isDriveFolder := driveDownloadCandidates(source)
	if isDriveFolder {
		return nil, "", "", fmt.Errorf("link folder Drive chưa tải trực tiếp được trong nhánh file đơn")
	}

	var lastErr error
	for _, candidate := range candidates {
		data, filename, contentType, err := a.downloadRemoteFile(candidate)
		if err == nil {
			if detected := detectImageMime(data); detected != "" {
				contentType = detected
			}
			return data, filename, contentType, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, "", "", lastErr
	}
	return nil, "", "", fmt.Errorf("link không hợp lệ")
}

func (a *app) resolveDriveFolderInputs(folderID, resourceKey, tmpDir string) ([]batchInputFile, error) {
	items, err := a.listDriveFolderImages(folderID, resourceKey)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("folder Drive không có ảnh JPG/PNG/WEBP công khai")
	}

	imageDir := filepath.Join(tmpDir, "drive")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		return nil, fmt.Errorf("không tạo được folder tạm")
	}

	seen := map[string]bool{}
	inputs := make([]batchInputFile, 0, len(items))
	for _, item := range items {
		data, filename, contentType, err := a.downloadDriveFile(item)
		if err != nil {
			return nil, fmt.Errorf("không tải được ảnh Drive %s: %w", item.Name, err)
		}
		if detected := detectImageMime(data); detected != "" {
			contentType = detected
		}

		name := fallbackDriveFilename(firstNonEmpty(item.Name, filename), contentType, item.ID)
		if !isSupportedImage(contentType, name) {
			continue
		}
		lowerName := strings.ToLower(name)
		if seen[lowerName] {
			return nil, fmt.Errorf("folder Drive có ảnh trùng tên: %s", name)
		}
		seen[lowerName] = true

		path := filepath.Join(imageDir, name)
		if err := os.WriteFile(path, data, 0644); err != nil {
			return nil, fmt.Errorf("không lưu được ảnh Drive tạm: %s", name)
		}
		inputs = append(inputs, batchInputFile{
			Path:        path,
			Name:        name,
			ContentType: contentTypeForFilename(name),
		})
	}

	sort.Slice(inputs, func(i, j int) bool {
		return strings.ToLower(inputs[i].Name) < strings.ToLower(inputs[j].Name)
	})
	return inputs, nil
}

func (a *app) listDriveFolderImages(folderID, resourceKey string) ([]driveFileItem, error) {
	if a.driveAPIKey != "" {
		items, err := a.listDriveFolderImagesWithAPI(folderID, resourceKey)
		if err == nil {
			return items, nil
		}
		log.Printf("drive api folder list failed, trying public fallback: %v", err)
	}

	items, err := a.scrapePublicDriveFolder(folderID, resourceKey)
	if err == nil && len(items) > 0 {
		return items, nil
	}
	if a.driveAPIKey == "" {
		return nil, fmt.Errorf("không liệt kê được folder Drive công khai; hãy bật Anyone with the link hoặc cấu hình GOOGLE_DRIVE_API_KEY để đọc folder ổn định")
	}
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("folder Drive không có ảnh JPG/PNG/WEBP công khai")
}

func (a *app) listDriveFolderImagesWithAPI(folderID, resourceKey string) ([]driveFileItem, error) {
	items := make([]driveFileItem, 0)
	pageToken := ""
	for {
		values := url.Values{}
		values.Set("key", a.driveAPIKey)
		values.Set("pageSize", "1000")
		values.Set("fields", "nextPageToken,files(id,name,mimeType,size,resourceKey)")
		values.Set("supportsAllDrives", "true")
		values.Set("includeItemsFromAllDrives", "true")
		values.Set("q", fmt.Sprintf("'%s' in parents and trashed = false", strings.ReplaceAll(folderID, "'", "\\'")))
		if pageToken != "" {
			values.Set("pageToken", pageToken)
		}

		req, err := http.NewRequest(http.MethodGet, "https://www.googleapis.com/drive/v3/files?"+values.Encode(), nil)
		if err != nil {
			return nil, fmt.Errorf("không tạo được request Drive API")
		}
		if resourceKey != "" {
			req.Header.Set("X-Goog-Drive-Resource-Keys", folderID+"/"+resourceKey)
		}
		resp, err := a.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("không gọi được Drive API")
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		closeErr := resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("không đọc được phản hồi Drive API")
		}
		if closeErr != nil {
			return nil, fmt.Errorf("không đóng được phản hồi Drive API")
		}

		var parsed driveFilesListResponse
		if err := json.Unmarshal(responseBody, &parsed); err != nil {
			return nil, fmt.Errorf("không đọc được JSON Drive API")
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			message := fmt.Sprintf("Drive API trả lỗi %d", resp.StatusCode)
			if parsed.Error != nil && parsed.Error.Message != "" {
				message = parsed.Error.Message
			}
			return nil, errors.New(message)
		}

		for _, item := range parsed.Files {
			if isDriveImage(item) {
				items = append(items, item)
			}
		}
		if parsed.NextPageToken == "" {
			break
		}
		pageToken = parsed.NextPageToken
	}
	return items, nil
}

func (a *app) scrapePublicDriveFolder(folderID, resourceKey string) ([]driveFileItem, error) {
	pages := driveFolderPageURLs(folderID, resourceKey)
	var lastErr error
	for _, pageURL := range pages {
		req, err := http.NewRequest(http.MethodGet, pageURL, nil)
		if err != nil {
			return nil, fmt.Errorf("link folder Drive không hợp lệ")
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 RingGlow/1.0")

		resp, err := a.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("không tải được trang folder Drive")
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
		closeErr := resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("folder Drive trả lỗi %d", resp.StatusCode)
			continue
		}
		if readErr != nil || closeErr != nil {
			lastErr = fmt.Errorf("không đọc được trang folder Drive")
			continue
		}

		items := parseDriveFolderHTML(string(data))
		if len(items) > 0 {
			return items, nil
		}
		lastErr = fmt.Errorf("không tìm thấy metadata ảnh trong trang folder Drive")
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("không đọc được folder Drive công khai")
}

func (a *app) downloadDriveFile(item driveFileItem) ([]byte, string, string, error) {
	name := strings.TrimSpace(item.Name)
	candidates := driveFileDownloadURLs(item.ID, item.ResourceKey)
	var lastErr error
	for _, candidate := range candidates {
		data, filename, contentType, err := a.downloadRemoteFile(candidate)
		if err == nil {
			return data, firstNonEmpty(name, filename), firstNonEmpty(item.MimeType, contentType), nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, "", "", lastErr
	}
	return nil, "", "", fmt.Errorf("không có link tải Drive hợp lệ")
}

func driveFolderPageURLs(folderID, resourceKey string) []string {
	escapedID := url.QueryEscape(folderID)
	pathID := url.PathEscape(folderID)
	values := url.Values{}
	if resourceKey != "" {
		values.Set("resourcekey", resourceKey)
	}
	suffix := ""
	if encoded := values.Encode(); encoded != "" {
		suffix = "?" + encoded
	}

	embedded := url.Values{}
	embedded.Set("id", folderID)
	if resourceKey != "" {
		embedded.Set("resourcekey", resourceKey)
	}

	return uniqueStrings([]string{
		"https://drive.google.com/drive/folders/" + pathID + suffix,
		"https://drive.google.com/drive/u/0/folders/" + pathID + suffix,
		"https://drive.google.com/embeddedfolderview?" + embedded.Encode() + "#grid",
		"https://drive.google.com/embeddedfolderview?id=" + escapedID + "#list",
	})
}

func parseDriveFolderHTML(text string) []driveFileItem {
	items := make([]driveFileItem, 0)
	seen := map[string]bool{}
	add := func(id, name, mimeType, resourceKey string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		name = strings.TrimSpace(name)
		if strings.HasPrefix(name, "drive-") {
			return
		}
		mimeType = cleanContentType(mimeType)
		if mimeType == "" && isSupportedImage("", name) {
			mimeType = contentTypeForFilename(name)
		}
		if !isSupportedImage(mimeType, name) {
			return
		}
		seen[id] = true
		if name == "" {
			name = "drive-" + id
		}
		items = append(items, driveFileItem{
			ID:          id,
			Name:        name,
			MimeType:    mimeType,
			ResourceKey: resourceKey,
		})
	}

	normalized := normalizeDriveHTML(text)
	if normalized != text {
		for _, item := range parseDriveFolderHTML(normalized) {
			add(item.ID, item.Name, item.MimeType, item.ResourceKey)
		}
	}

	jsonPattern := regexp.MustCompile(`\["([A-Za-z0-9_-]{20,})","((?:\\.|[^"\\])*)"(?:[^\]]{0,900})"(image/(?:jpeg|png|webp))"`)
	for _, match := range jsonPattern.FindAllStringSubmatch(text, -1) {
		add(match[1], unescapeDriveString(match[2]), match[3], "")
	}

	entryPattern := regexp.MustCompile(`(?s)<div class="flip-entry"[^>]*id="entry-([A-Za-z0-9_-]{20,})".*?<img src="https://drive-thirdparty\.googleusercontent\.com/16/type/(image/(?:jpeg|png|webp))"[^>]*>.*?<div class="flip-entry-title">([^<]+)</div>`)
	for _, match := range entryPattern.FindAllStringSubmatch(text, -1) {
		add(match[1], html.UnescapeString(match[3]), match[2], "")
	}

	fileLinkPattern := regexp.MustCompile(`https://drive\.google\.com/(?:file/d/|open\?id=)([A-Za-z0-9_-]{20,})(?:[^"'<>]*)`)
	for _, match := range fileLinkPattern.FindAllStringSubmatch(text, -1) {
		id := match[1]
		resourceKey := resourceKeyFromText(match[0])
		name := nearbyImageFilename(text, id)
		add(id, name, contentTypeForFilename(name), resourceKey)
	}

	return items
}

func normalizeDriveHTML(text string) string {
	replacements := []struct {
		old string
		new string
	}{
		{`\/`, `/`},
		{`\u003d`, `=`},
		{`\u0026`, `&`},
		{`\u003c`, `<`},
		{`\u003e`, `>`},
		{`\u002F`, `/`},
	}
	for _, replacement := range replacements {
		text = strings.ReplaceAll(text, replacement.old, replacement.new)
	}
	return html.UnescapeString(text)
}

func nearbyImageFilename(text, id string) string {
	index := strings.Index(text, id)
	if index < 0 {
		return ""
	}
	start := index - 1800
	if start < 0 {
		start = 0
	}
	end := index + 1800
	if end > len(text) {
		end = len(text)
	}
	window := text[start:end]
	filenamePattern := regexp.MustCompile(`(?i)((?:[\p{L}\p{N}_ .()\-]+)\.(?:jpe?g|png|webp))`)
	matches := filenamePattern.FindAllStringSubmatch(window, -1)
	if len(matches) == 0 {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(unescapeDriveString(matches[len(matches)-1][1])))
}

func resourceKeyFromText(value string) string {
	parsed, err := url.Parse(html.UnescapeString(value))
	if err != nil {
		return ""
	}
	return parsed.Query().Get("resourcekey")
}

func (a *app) downloadRemoteFile(rawURL string) ([]byte, string, string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("link không hợp lệ")
	}
	req.Header.Set("User-Agent", "RingGlow/1.0")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, "", "", fmt.Errorf("không tải được link nguồn")
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", "", fmt.Errorf("link nguồn trả về lỗi %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBatchArchiveBytes+1))
	if err != nil {
		return nil, "", "", fmt.Errorf("không đọc được dữ liệu từ link")
	}
	if len(data) > maxBatchArchiveBytes {
		return nil, "", "", fmt.Errorf("file tải về lớn hơn 500MB")
	}

	contentType := cleanContentType(resp.Header.Get("Content-Type"))
	filename := filenameFromResponse(resp, rawURL)
	if strings.HasPrefix(contentType, "text/html") {
		return nil, "", "", fmt.Errorf("link này chưa trả file ảnh trực tiếp; với Google Drive hãy bật quyền Anyone with the link và dùng link file ảnh JPG/PNG/WEBP")
	}

	return data, filename, contentType, nil
}

func extractBatchArchive(data []byte, tmpDir string) ([]batchInputFile, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("không đọc được file ZIP")
	}

	imageDir := filepath.Join(tmpDir, "images")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		return nil, fmt.Errorf("không tạo được folder tạm")
	}

	seen := map[string]bool{}
	inputs := make([]batchInputFile, 0)
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(file.Name)
		if !isSupportedImage("", name) {
			continue
		}
		lowerName := strings.ToLower(name)
		if seen[lowerName] {
			return nil, fmt.Errorf("file ZIP có ảnh trùng tên: %s", name)
		}
		seen[lowerName] = true

		src, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("không đọc được ảnh trong ZIP: %s", name)
		}
		imageBytes, err := io.ReadAll(io.LimitReader(src, maxImageBytes+1))
		closeErr := src.Close()
		if err != nil {
			return nil, fmt.Errorf("không đọc được ảnh trong ZIP: %s", name)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("không đóng được ảnh trong ZIP: %s", name)
		}
		if len(imageBytes) > maxImageBytes {
			return nil, fmt.Errorf("ảnh trong ZIP lớn hơn 50MB: %s", name)
		}

		path := filepath.Join(imageDir, name)
		if err := os.WriteFile(path, imageBytes, 0644); err != nil {
			return nil, fmt.Errorf("không lưu được ảnh tạm: %s", name)
		}
		inputs = append(inputs, batchInputFile{
			Path:        path,
			Name:        name,
			ContentType: contentTypeForFilename(name),
		})
	}

	sort.Slice(inputs, func(i, j int) bool {
		return strings.ToLower(inputs[i].Name) < strings.ToLower(inputs[j].Name)
	})
	return inputs, nil
}

func (a *app) finalizeImageBytes(data []byte, currentMime, targetFormat string) ([]byte, string, error) {
	targetMime := mimeForFormat(targetFormat)
	currentMime = cleanContentType(firstNonEmpty(currentMime, detectImageMime(data)))

	if targetFormat == "webp" {
		if currentMime == targetMime {
			return data, targetMime, nil
		}
		return nil, "", fmt.Errorf("API chưa trả về WEBP, không thể giữ đúng định dạng WEBP cho file này")
	}

	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("không chuyển được ảnh kết quả sang đúng định dạng")
	}
	locked := applyGoldPalette(decoded, a.referencePalette)

	outputBytes, err := encodeImageBytes(locked, targetFormat)
	if err != nil {
		return nil, "", err
	}
	return outputBytes, targetMime, nil
}

func encodeImageBytes(src image.Image, targetFormat string) ([]byte, error) {
	var buffer bytes.Buffer
	switch targetFormat {
	case "jpeg":
		if err := jpeg.Encode(&buffer, flattenOnWhite(src), &jpeg.Options{Quality: 95}); err != nil {
			return nil, fmt.Errorf("không xuất được JPEG")
		}
	case "png":
		if err := png.Encode(&buffer, src); err != nil {
			return nil, fmt.Errorf("không xuất được PNG")
		}
	default:
		return nil, fmt.Errorf("định dạng ảnh không được hỗ trợ")
	}
	return buffer.Bytes(), nil
}

func applyGoldPalette(src image.Image, palette goldPalette) image.Image {
	if !palette.Ready {
		palette = defaultGoldPalette()
	}

	stats := analyzeGoldImage(src)
	if stats.Count < 24 {
		return src
	}

	bounds := src.Bounds()
	dst := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixel := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
			h, s, l := rgbToHSL(pixel)
			confidence, ok := goldPixelConfidence(pixel, h, s, l)
			if !ok {
				dst.SetNRGBA(x, y, pixel)
				continue
			}

			strength := clampFloat(confidence*0.86, 0.38, 0.86)
			targetLightness := palette.Lightness + (l-stats.Lightness)*0.88
			targetLightness = clampFloat(targetLightness, 0.18, 0.91)
			targetSaturation := palette.Saturation + (0.62-targetLightness)*0.15
			targetSaturation = clampFloat(targetSaturation, 0.32, 0.66)

			lockedH := mixHue(h, palette.Hue, strength*0.92)
			lockedS := lerpFloat(s, targetSaturation, strength*0.82)
			lockedL := lerpFloat(l, targetLightness, strength*0.82)
			glossAmount := clampFloat((l-stats.Lightness)/0.24, 0, 1) * confidence
			midtoneAmount := clampFloat((0.76-l)/0.42, 0, 1) * confidence
			lockedL = clampFloat(lockedL-midtoneAmount*0.012+glossAmount*0.032, 0, 0.94)
			lockedS = clampFloat(lockedS+midtoneAmount*0.030+glossAmount*0.014, 0, 0.68)

			dst.SetNRGBA(x, y, hslToRGB(lockedH, lockedS, lockedL, pixel.A))
		}
	}
	return dst
}

func analyzeGoldPalette(data []byte) goldPalette {
	if len(data) == 0 {
		return defaultGoldPalette()
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return defaultGoldPalette()
	}

	stats := analyzeGoldImage(img)
	if stats.Count < 24 {
		return defaultGoldPalette()
	}

	return goldPalette{
		Hue:        stats.Hue,
		Saturation: clampFloat(stats.Saturation+0.070, 0.40, 0.63),
		Lightness:  clampFloat(stats.Lightness-0.018, 0.535, 0.67),
		Ready:      true,
	}
}

func analyzeGoldImage(img image.Image) goldImageStats {
	bounds := img.Bounds()
	var hueSin, hueCos, saturation, lightness, weightSum float64
	count := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixel := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			h, s, l := rgbToHSL(pixel)
			confidence, ok := goldPixelConfidence(pixel, h, s, l)
			if !ok {
				continue
			}

			weight := confidence
			angle := h * 2 * math.Pi
			hueSin += math.Sin(angle) * weight
			hueCos += math.Cos(angle) * weight
			saturation += s * weight
			lightness += l * weight
			weightSum += weight
			count++
		}
	}

	if count == 0 || weightSum == 0 {
		return goldImageStats{}
	}

	hue := math.Atan2(hueSin, hueCos) / (2 * math.Pi)
	if hue < 0 {
		hue += 1
	}
	return goldImageStats{
		Count:      count,
		Hue:        hue,
		Saturation: saturation / weightSum,
		Lightness:  lightness / weightSum,
	}
}

func defaultGoldPalette() goldPalette {
	return goldPalette{
		Hue:        45.0 / 360.0,
		Saturation: 0.535,
		Lightness:  0.612,
		Ready:      true,
	}
}

func goldPixelConfidence(pixel color.NRGBA, h, s, l float64) (float64, bool) {
	if pixel.A < 12 || l < 0.13 || l > 0.96 {
		return 0, false
	}

	hue := h * 360
	if hue < 18 || hue > 76 {
		return 0, false
	}

	warmOrder := int(pixel.R) >= int(pixel.G)-24 &&
		int(pixel.G) > int(pixel.B)+6 &&
		int(pixel.R) > int(pixel.B)+18
	if !warmOrder {
		return 0, false
	}

	if s < 0.08 && !(pixel.R > 145 && pixel.G > 120 && pixel.B < 120) {
		return 0, false
	}

	hueCenter := 45.0
	hueDistance := math.Abs(hue - hueCenter)
	hueConfidence := clampFloat(1-(hueDistance/34), 0, 1)
	saturationConfidence := clampFloat((s-0.06)/0.34, 0.18, 1)
	lightnessConfidence := 1.0
	if l > 0.82 {
		lightnessConfidence = clampFloat((0.96-l)/0.14, 0.28, 1)
	} else if l < 0.25 {
		lightnessConfidence = clampFloat((l-0.13)/0.12, 0.28, 1)
	}

	confidence := clampFloat(0.20+hueConfidence*0.42+saturationConfidence*0.28+lightnessConfidence*0.10, 0, 1)
	if confidence < 0.25 {
		return 0, false
	}
	return confidence, true
}

func rgbToHSL(pixel color.NRGBA) (float64, float64, float64) {
	r := float64(pixel.R) / 255
	g := float64(pixel.G) / 255
	b := float64(pixel.B) / 255

	maxValue := math.Max(r, math.Max(g, b))
	minValue := math.Min(r, math.Min(g, b))
	l := (maxValue + minValue) / 2
	if maxValue == minValue {
		return 0, 0, l
	}

	delta := maxValue - minValue
	s := delta / (1 - math.Abs(2*l-1))
	var h float64
	switch maxValue {
	case r:
		h = math.Mod((g-b)/delta, 6)
	case g:
		h = (b-r)/delta + 2
	default:
		h = (r-g)/delta + 4
	}
	h /= 6
	if h < 0 {
		h += 1
	}
	return h, s, l
}

func hslToRGB(h, s, l float64, alpha uint8) color.NRGBA {
	h = math.Mod(h, 1)
	if h < 0 {
		h += 1
	}
	s = clampFloat(s, 0, 1)
	l = clampFloat(l, 0, 1)

	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h*6, 2)-1))
	m := l - c/2

	var r, g, b float64
	switch {
	case h < 1.0/6.0:
		r, g, b = c, x, 0
	case h < 2.0/6.0:
		r, g, b = x, c, 0
	case h < 3.0/6.0:
		r, g, b = 0, c, x
	case h < 4.0/6.0:
		r, g, b = 0, x, c
	case h < 5.0/6.0:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}

	return color.NRGBA{
		R: uint8(math.Round(clampFloat(r+m, 0, 1) * 255)),
		G: uint8(math.Round(clampFloat(g+m, 0, 1) * 255)),
		B: uint8(math.Round(clampFloat(b+m, 0, 1) * 255)),
		A: alpha,
	}
}

func mixHue(from, to, amount float64) float64 {
	amount = clampFloat(amount, 0, 1)
	delta := math.Mod(to-from+1.5, 1) - 0.5
	result := from + delta*amount
	if result < 0 {
		result += 1
	}
	return math.Mod(result, 1)
}

func lerpFloat(from, to, amount float64) float64 {
	return from + (to-from)*clampFloat(amount, 0, 1)
}

func clampFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func flattenOnWhite(src image.Image) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, &image.Uniform{color.White}, image.Point{}, draw.Src)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Over)
	return dst
}

func writeImagePart(writer *multipart.Writer, fieldName, filename, contentType string, image []byte) error {
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, escapeQuotes(filename)))
	partHeader.Set("Content-Type", contentType)

	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return err
	}
	_, err = part.Write(image)
	return err
}

func loadGoldReference() ([]byte, string, string) {
	path := goldReferencePath()
	if path == "" {
		return nil, "", ""
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("gold reference image unavailable: %v", err)
		return nil, "", ""
	}
	if len(data) == 0 || len(data) > maxImageBytes {
		log.Printf("gold reference image is empty or too large: %s", path)
		return nil, "", ""
	}

	filename := filepath.Base(path)
	contentType := contentTypeForFilename(filename)
	if !isSupportedImage(contentType, filename) {
		log.Printf("gold reference image must be jpg, png, or webp: %s", path)
		return nil, "", ""
	}

	return data, filename, contentType
}

func goldReferencePath() string {
	if path := strings.TrimSpace(os.Getenv("GOLD_REFERENCE_IMAGE")); path != "" {
		return path
	}

	candidates := []string{
		filepath.Join("..", "frontend", "public", "examples", "gold-reference.jpg"),
		filepath.Join("frontend", "public", "examples", "gold-reference.jpg"),
		"gold-reference.jpg",
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func contentTypeForFilename(filename string) string {
	name := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".webp"):
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

func outputFormatForFilename(filename string) string {
	name := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(name, ".png"):
		return "png"
	case strings.HasSuffix(name, ".webp"):
		return "webp"
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
		return "jpeg"
	default:
		return ""
	}
}

func detectImageMime(data []byte) string {
	contentType := cleanContentType(http.DetectContentType(data))
	if contentType == "image/jpeg" || contentType == "image/png" || contentType == "image/webp" {
		return contentType
	}
	return ""
}

func cleanContentType(contentType string) string {
	if contentType == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	}
	return strings.ToLower(mediaType)
}

func isHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func driveDownloadCandidates(rawURL string) ([]string, bool) {
	id, resourceKey, isDrive, isFolder := driveFileID(rawURL)
	if !isDrive || id == "" {
		return []string{rawURL}, false
	}
	if isFolder {
		return []string{rawURL}, true
	}

	return append([]string{rawURL}, driveFileDownloadURLs(id, resourceKey)...), false
}

func driveFileID(rawURL string) (string, string, bool, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.Contains(strings.ToLower(parsed.Host), "drive.google.com") {
		return "", "", false, false
	}

	id := parsed.Query().Get("id")
	resourceKey := parsed.Query().Get("resourcekey")
	isFolder := false
	if id == "" {
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		for i, part := range parts {
			if part == "folders" && i+1 < len(parts) {
				id = parts[i+1]
				isFolder = true
				break
			}
			if part == "d" && i+1 < len(parts) {
				id = parts[i+1]
				break
			}
		}
	}
	return id, resourceKey, true, isFolder
}

func driveFileDownloadURLs(id, resourceKey string) []string {
	escapedID := url.QueryEscape(id)
	resourceSuffix := ""
	if resourceKey != "" {
		resourceSuffix = "&resourcekey=" + url.QueryEscape(resourceKey)
	}
	return uniqueStrings([]string{
		"https://drive.google.com/uc?export=download&id=" + escapedID + resourceSuffix,
		"https://drive.usercontent.google.com/download?id=" + escapedID + "&export=download&confirm=t" + resourceSuffix,
		"https://drive.google.com/thumbnail?id=" + escapedID + "&sz=w4096" + resourceSuffix,
	})
}

func isDriveImage(item driveFileItem) bool {
	mimeType := cleanContentType(item.MimeType)
	return mimeType == "image/jpeg" ||
		mimeType == "image/png" ||
		mimeType == "image/webp" ||
		isSupportedImage(mimeType, item.Name)
}

func fallbackDriveFilename(filename, contentType, id string) string {
	name := strings.TrimSpace(filepath.Base(filename))
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "drive-" + id
	}
	if isSupportedImage(contentType, name) {
		return name
	}

	switch cleanContentType(contentType) {
	case "image/png":
		return name + ".png"
	case "image/webp":
		return name + ".webp"
	default:
		return name + ".jpg"
	}
}

func unescapeDriveString(value string) string {
	unquoted, err := strconv.Unquote(`"` + value + `"`)
	if err == nil {
		return html.UnescapeString(unquoted)
	}
	return html.UnescapeString(strings.ReplaceAll(value, `\"`, `"`))
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func isZipArchive(data []byte, filename, contentType string) bool {
	contentType = cleanContentType(contentType)
	name := strings.ToLower(filename)
	return contentType == "application/zip" ||
		contentType == "application/x-zip-compressed" ||
		strings.HasSuffix(name, ".zip") ||
		bytes.HasPrefix(data, []byte{'P', 'K', 0x03, 0x04})
}

func filenameFromResponse(resp *http.Response, rawURL string) string {
	if disposition := resp.Header.Get("Content-Disposition"); disposition != "" {
		if _, params, err := mime.ParseMediaType(disposition); err == nil {
			if filename := strings.TrimSpace(params["filename"]); filename != "" {
				return filepath.Base(filename)
			}
		}
	}

	parsed, err := url.Parse(rawURL)
	if err == nil {
		if base := path.Base(parsed.Path); base != "." && base != "/" && base != "" {
			return filepath.Base(base)
		}
	}
	return "ringglow-source"
}

func fallbackRemoteFilename(filename, contentType string) string {
	if isSupportedImage("", filename) {
		return filepath.Base(filename)
	}

	switch cleanContentType(contentType) {
	case "image/png":
		return "ringglow-source.png"
	case "image/webp":
		return "ringglow-source.webp"
	default:
		return "ringglow-source.jpg"
	}
}

func shouldRetryWithoutReference(err error) bool {
	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		return false
	}

	message := strings.ToLower(apiErr.message)
	if apiErr.providerStatus == http.StatusBadRequest || apiErr.providerStatus == http.StatusUnprocessableEntity || apiErr.providerStatus == http.StatusOK {
		return strings.Contains(message, "image") ||
			strings.Contains(message, "field") ||
			strings.Contains(message, "array") ||
			strings.Contains(message, "invalid") ||
			strings.Contains(message, "unsupported")
	}
	return false
}

type apiError struct {
	status         int
	providerStatus int
	message        string
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
