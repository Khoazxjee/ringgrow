import { type ChangeEvent, type DragEvent, type ReactNode, useEffect, useMemo, useRef, useState } from 'react'
import {
  Camera,
  CheckCircle2,
  Download,
  Gem,
  LoaderCircle,
  RotateCcw,
  Settings2,
  ShieldCheck,
  Sparkles,
  WandSparkles,
} from 'lucide-react'
import './App.css'

type EnhanceResult = {
  image: string
  mimeType: string
  model: string
  prompt: string
  quality?: string
  size?: string
  outputFormat?: string
}

type Quality = 'high' | 'medium' | 'auto'
type Size = 'auto' | '1536x1024' | '1024x1024' | '1024x1536'
type OutputFormat = 'png' | 'jpeg' | 'webp'

const API_BASE = (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? '/api'
const GOLD_REFERENCE = '/examples/gold-reference.jpg'

const qualityOptions: Array<{ label: string; value: Quality }> = [
  { label: 'Cao', value: 'high' },
  { label: 'Vừa', value: 'medium' },
  { label: 'Tự động', value: 'auto' },
]

const sizeOptions: Array<{ label: string; value: Size }> = [
  { label: 'Tự động', value: 'auto' },
  { label: 'Ngang', value: '1536x1024' },
  { label: 'Vuông', value: '1024x1024' },
]

const formatOptions: Array<{ label: string; value: OutputFormat }> = [
  { label: 'PNG', value: 'png' },
  { label: 'JPEG', value: 'jpeg' },
  { label: 'WEBP', value: 'webp' },
]

const highlights = [
  { icon: ShieldCheck, title: 'Giữ chi tiết', text: 'Hoạ tiết và đá không đổi' },
  { icon: Gem, title: 'Vàng tự nhiên', text: 'Tông 18K cân bằng' },
  { icon: Camera, title: 'Studio light', text: 'Sạch, nét, không giả' },
]

function App() {
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [beforeUrl, setBeforeUrl] = useState('')
  const [afterUrl, setAfterUrl] = useState('')
  const [quality, setQuality] = useState<Quality>('high')
  const [size, setSize] = useState<Size>('auto')
  const [outputFormat, setOutputFormat] = useState<OutputFormat>('png')
  const [isDragging, setIsDragging] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState('')
  const [downloadToast, setDownloadToast] = useState(false)
  const [result, setResult] = useState<EnhanceResult | null>(null)

  useEffect(() => {
    return () => {
      if (beforeUrl.startsWith('blob:')) {
        URL.revokeObjectURL(beforeUrl)
      }
    }
  }, [beforeUrl])

  useEffect(() => {
    if (!downloadToast) {
      return
    }

    const timeout = window.setTimeout(() => setDownloadToast(false), 2600)
    return () => window.clearTimeout(timeout)
  }, [downloadToast])

  const selectedName = useMemo(() => file?.name ?? 'Chưa chọn ảnh', [file])
  const endpoint = `${API_BASE.replace(/\/$/, '')}/enhance`
  const canGenerate = !isLoading && Boolean(file && beforeUrl)

  function chooseFile(nextFile: File) {
    if (!nextFile.type.startsWith('image/')) {
      setError('File phải là ảnh JPG, PNG hoặc WEBP.')
      return
    }

    setFile(nextFile)
    setBeforeUrl(URL.createObjectURL(nextFile))
    setAfterUrl('')
    setResult(null)
    setError('')
  }

  function onInputChange(event: ChangeEvent<HTMLInputElement>) {
    const nextFile = event.currentTarget.files?.[0]
    if (nextFile) {
      chooseFile(nextFile)
    }
    event.currentTarget.value = ''
  }

  function onDrop(event: DragEvent<HTMLButtonElement>) {
    event.preventDefault()
    setIsDragging(false)
    const nextFile = event.dataTransfer.files?.[0]
    if (nextFile) {
      chooseFile(nextFile)
    }
  }

  async function enhanceImage() {
    if (!file) {
      setError('Vui lòng upload file ảnh.')
      return
    }

    setIsLoading(true)
    setError('')
    setResult(null)

    try {
      const formData = new FormData()
      formData.append('image', file)
      formData.append('quality', quality)
      formData.append('size', size)
      formData.append('output_format', outputFormat)

      const response = await fetch(endpoint, {
        method: 'POST',
        body: formData,
      })
      const payload = await response.json()

      if (!response.ok) {
        throw new Error(payload.error ?? 'Không xử lý được ảnh.')
      }

      setAfterUrl(payload.image)
      setResult(payload)
    } catch (caught) {
      setAfterUrl('')
      setError(caught instanceof Error ? caught.message : 'Không xử lý được ảnh.')
    } finally {
      setIsLoading(false)
    }
  }

  function resetImage() {
    setFile(null)
    setBeforeUrl('')
    setAfterUrl('')
    setResult(null)
    setError('')
  }

  function downloadResult() {
    if (!afterUrl) {
      return
    }
    const extension = result?.outputFormat === 'jpeg' ? 'jpg' : result?.outputFormat ?? outputFormat
    const anchor = document.createElement('a')
    anchor.href = afterUrl
    anchor.download = `ringglow-studio-${Date.now()}.${extension}`
    anchor.click()
    setDownloadToast(true)
  }

  return (
    <div className="app-shell">
      <header className="top-region">
        <nav className="top-nav" aria-label="Primary">
          <div className="brand">
            <span className="brand-mark" aria-hidden="true" />
            <span>RingGlow</span>
          </div>
          <div className="nav-actions" aria-label="Pipeline status">
            <span className="status-pill muted">Studio retouch</span>
            <span className="status-pill">gpt-image-2</span>
          </div>
        </nav>
      </header>

      <main className="studio-page">
        <section className="studio-hero" aria-labelledby="studio-title">
          <div>
            <span className="eyebrow">Jewelry AI Studio</span>
            <h1 id="studio-title">Làm màu vàng tự nhiên hơn, ảnh nhẫn vẫn sắc nét.</h1>
            <p>Biến ảnh nhẫn đầu vào thành ảnh sản phẩm sáng sạch, đúng cảm giác chụp trong studio.</p>
          </div>
          <div className="highlight-row" aria-label="Edit guarantees">
            {highlights.map((item) => (
              <div className="highlight-item" key={item.title}>
                <item.icon size={18} strokeWidth={1.8} />
                <div>
                  <strong>{item.title}</strong>
                  <span>{item.text}</span>
                </div>
              </div>
            ))}
          </div>
        </section>

        <div className="studio-grid">
          <aside className="control-panel" aria-label="Image controls">
            <div className="panel-title">
              <Settings2 size={18} strokeWidth={1.8} />
              <div>
                <h2>Thiết lập</h2>
                <p>Chọn cấu hình xuất cho ảnh sau xử lý.</p>
              </div>
            </div>

            <div className="reference-card">
              <div>
                <span>Tông mục tiêu</span>
                <strong>Vàng studio sáng tự nhiên</strong>
                <small>Champagne highlight, honey-gold midtone</small>
              </div>
              <img src={GOLD_REFERENCE} alt="Reference gold tone" />
            </div>

            <div className="settings-stack">
              <ControlGroup
                title="Chất lượng"
                note="Nên dùng Cao"
                ariaLabel="Quality"
                options={qualityOptions}
                value={quality}
                onChange={setQuality}
              />
              <ControlGroup
                title="Khung ảnh"
                note="Auto cho ảnh upload"
                ariaLabel="Size"
                options={sizeOptions}
                value={size}
                onChange={setSize}
              />
              <ControlGroup
                title="Định dạng"
                note="PNG giữ chi tiết tốt"
                ariaLabel="Format"
                options={formatOptions}
                value={outputFormat}
                onChange={setOutputFormat}
              />
            </div>

            {error ? <div className="error-message">{error}</div> : null}

            <div className="action-bar">
              <button className="button primary" type="button" disabled={!canGenerate} onClick={enhanceImage}>
                {isLoading ? <LoaderCircle className="spin" size={18} /> : <WandSparkles size={18} />}
                {isLoading ? 'Đang xử lý' : 'Tạo ảnh vàng'}
              </button>
              <button className="button tertiary icon-only" type="button" onClick={resetImage} aria-label="Xóa ảnh">
                <RotateCcw size={18} />
              </button>
            </div>
          </aside>

          <section className="preview-area" aria-label="Before and after">
            <div className="preview-heading">
              <div>
                <span className="eyebrow">Preview</span>
                <h2>So sánh trước và sau</h2>
              </div>
              {beforeUrl ? (
                <div className="result-meta">
                  <span>{result?.quality ?? quality}</span>
                  <span>{result?.size ?? size}</span>
                  <span>{result?.outputFormat ?? outputFormat}</span>
                </div>
              ) : null}
            </div>

            <input
              ref={fileInputRef}
              className="file-input"
              type="file"
              accept="image/jpeg,image/png,image/webp"
              onChange={onInputChange}
            />

            {!beforeUrl ? (
              <button
                className={`upload-only-card ${isDragging ? 'is-dragging' : ''}`}
                type="button"
                onClick={() => fileInputRef.current?.click()}
                onDragOver={(event) => {
                  event.preventDefault()
                  setIsDragging(true)
                }}
                onDragLeave={() => setIsDragging(false)}
                onDrop={onDrop}
              >
                Upload file ảnh
              </button>
            ) : (
              <>
                <div className="comparison">
                  <ImageCard label="Before" title="Ảnh gốc" badge={selectedName}>
                    <img src={beforeUrl} alt="Original ring" />
                    <button className="change-image-button" type="button" onClick={() => fileInputRef.current?.click()}>
                      Đổi ảnh
                    </button>
                  </ImageCard>

                  <ImageCard
                    className="after-card"
                    label="After"
                    title="Studio gold"
                    badge={
                      result ? (
                        <>
                          <CheckCircle2 size={14} />
                          {result.model}
                        </>
                      ) : (
                        'Ready'
                      )
                    }
                  >
                    {afterUrl ? (
                      <img src={afterUrl} alt="Enhanced gold ring" />
                    ) : (
                      <div className="empty-state">
                        <span className="empty-icon" aria-hidden="true">
                          {isLoading ? <LoaderCircle className="spin" size={32} /> : <Sparkles size={32} />}
                        </span>
                        <strong>{isLoading ? 'Đang dựng ánh sáng studio' : 'Kết quả sẽ hiện ở đây'}</strong>
                        <span>{isLoading ? 'Backend đang giữ nguyên chi tiết nhẫn.' : 'Bấm tạo ảnh để xem bản vàng tự nhiên hơn.'}</span>
                      </div>
                    )}
                  </ImageCard>
                </div>

                <div className="download-strip">
                  <div>
                    <strong>{afterUrl ? 'Ảnh đã sẵn sàng' : 'Chưa có ảnh sau xử lý'}</strong>
                    <span>{afterUrl ? 'Bạn có thể tải kết quả về máy.' : 'Kết quả xuất ra sẽ dùng thiết lập bên trái.'}</span>
                  </div>
                  <button className="button tertiary compact" type="button" disabled={!afterUrl} onClick={downloadResult}>
                    <Download size={16} />
                    Tải ảnh
                  </button>
                </div>
              </>
            )}
          </section>
        </div>
      </main>

      {downloadToast ? (
        <div className="download-toast" role="status" aria-live="polite">
          <CheckCircle2 size={18} />
          <div>
            <strong>Đã tải ảnh</strong>
            <span>File kết quả đã được lưu về máy.</span>
          </div>
        </div>
      ) : null}
    </div>
  )
}

function ControlGroup<T extends string>({
  title,
  note,
  ariaLabel,
  options,
  value,
  onChange,
}: {
  title: string
  note: string
  ariaLabel: string
  options: Array<{ label: string; value: T }>
  value: T
  onChange: (value: T) => void
}) {
  return (
    <div className="control-group">
      <div className="control-head">
        <span>{title}</span>
        <small>{note}</small>
      </div>
      <div className="segmented" role="group" aria-label={ariaLabel}>
        {options.map((option) => (
          <button
            key={option.value}
            type="button"
            className={value === option.value ? 'active' : ''}
            onClick={() => onChange(option.value)}
          >
            {option.label}
          </button>
        ))}
      </div>
    </div>
  )
}

function ImageCard({
  className = '',
  label,
  title,
  badge,
  children,
}: {
  className?: string
  label: string
  title: string
  badge: ReactNode
  children: ReactNode
}) {
  return (
    <article className={`image-card ${className}`}>
      <div className="card-heading">
        <div>
          <span className="kicker">{label}</span>
          <h3>{title}</h3>
        </div>
        <span className={`file-pill ${className ? 'ready' : ''}`}>{badge}</span>
      </div>
      <div className="image-frame">{children}</div>
    </article>
  )
}

export default App
