import { type ChangeEvent, type DragEvent, useEffect, useMemo, useRef, useState } from 'react'
import {
  CheckCircle2,
  Download,
  Image as ImageIcon,
  LoaderCircle,
  RotateCcw,
  Sparkles,
  UploadCloud,
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
const SAMPLE_BEFORE = '/examples/ring-before.jpg'

const qualityOptions: Array<{ label: string; value: Quality }> = [
  { label: 'High', value: 'high' },
  { label: 'Medium', value: 'medium' },
  { label: 'Auto', value: 'auto' },
]

const sizeOptions: Array<{ label: string; value: Size }> = [
  { label: 'Auto', value: 'auto' },
  { label: 'Wide', value: '1536x1024' },
  { label: 'Square', value: '1024x1024' },
]

const formatOptions: Array<{ label: string; value: OutputFormat }> = [
  { label: 'PNG', value: 'png' },
  { label: 'JPEG', value: 'jpeg' },
  { label: 'WEBP', value: 'webp' },
]

function App() {
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [beforeUrl, setBeforeUrl] = useState(SAMPLE_BEFORE)
  const [afterUrl, setAfterUrl] = useState('')
  const [quality, setQuality] = useState<Quality>('high')
  const [size, setSize] = useState<Size>('auto')
  const [outputFormat, setOutputFormat] = useState<OutputFormat>('png')
  const [isDragging, setIsDragging] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<EnhanceResult | null>(null)

  useEffect(() => {
    return () => {
      if (beforeUrl.startsWith('blob:')) {
        URL.revokeObjectURL(beforeUrl)
      }
    }
  }, [beforeUrl])

  const selectedName = useMemo(() => file?.name ?? 'ring-before.jpg', [file])
  const endpoint = `${API_BASE.replace(/\/$/, '')}/enhance`
  const canGenerate = !isLoading && Boolean(beforeUrl)

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

  function onDrop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault()
    setIsDragging(false)
    const nextFile = event.dataTransfer.files?.[0]
    if (nextFile) {
      chooseFile(nextFile)
    }
  }

  async function sampleAsFile() {
    const response = await fetch(SAMPLE_BEFORE)
    const blob = await response.blob()
    return new File([blob], 'ring-before.jpg', { type: blob.type || 'image/jpeg' })
  }

  async function enhanceImage() {
    setIsLoading(true)
    setError('')
    setResult(null)

    try {
      const imageFile = file ?? (await sampleAsFile())
      const formData = new FormData()
      formData.append('image', imageFile)
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

  function resetToSample() {
    setFile(null)
    setBeforeUrl(SAMPLE_BEFORE)
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
  }

  return (
    <div className="app-shell">
      <header className="top-region">
        <div className="promo-banner">
          <span>RingGlow Studio</span>
          <span>GPT Image 2 edit pipeline</span>
        </div>
        <nav className="top-nav" aria-label="Primary">
          <div className="brand">
            <span className="brand-dot" aria-hidden="true" />
            <span>RingGlow</span>
          </div>
          <div className="nav-actions">
            <span className="badge beta">Studio</span>
            <span className="badge success">Preserve detail</span>
          </div>
        </nav>
      </header>

      <main className="workspace">
        <section className="control-panel" aria-label="Image controls">
          <div className="panel-heading">
            <span className="badge new">Gold edit</span>
            <h1>Nhẫn vàng chuẩn studio</h1>
            <p>Giữ nguyên hoạ tiết, hoa văn, đá và hình dáng.</p>
          </div>

          <div
            className={`dropzone ${isDragging ? 'is-dragging' : ''}`}
            onDragOver={(event) => {
              event.preventDefault()
              setIsDragging(true)
            }}
            onDragLeave={() => setIsDragging(false)}
            onDrop={onDrop}
          >
            <UploadCloud size={24} strokeWidth={1.8} />
            <div>
              <strong>{selectedName}</strong>
              <span>JPG, PNG, WEBP dưới 50MB</span>
            </div>
            <button className="button secondary" type="button" onClick={() => fileInputRef.current?.click()}>
              <ImageIcon size={17} />
              Chọn ảnh
            </button>
            <input
              ref={fileInputRef}
              className="file-input"
              type="file"
              accept="image/jpeg,image/png,image/webp"
              onChange={onInputChange}
            />
          </div>

          <div className="control-group">
            <div className="control-label">Quality</div>
            <div className="segmented" role="group" aria-label="Quality">
              {qualityOptions.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  className={quality === option.value ? 'active' : ''}
                  onClick={() => setQuality(option.value)}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </div>

          <div className="control-group">
            <div className="control-label">Size</div>
            <div className="segmented" role="group" aria-label="Size">
              {sizeOptions.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  className={size === option.value ? 'active' : ''}
                  onClick={() => setSize(option.value)}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </div>

          <div className="control-group">
            <div className="control-label">Format</div>
            <div className="segmented" role="group" aria-label="Format">
              {formatOptions.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  className={outputFormat === option.value ? 'active' : ''}
                  onClick={() => setOutputFormat(option.value)}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </div>

          {error ? <div className="error-message">{error}</div> : null}

          <div className="button-row">
            <button className="button primary" type="button" disabled={!canGenerate} onClick={enhanceImage}>
              {isLoading ? <LoaderCircle className="spin" size={18} /> : <WandSparkles size={18} />}
              {isLoading ? 'Đang xử lý' : 'Tạo ảnh vàng'}
            </button>
            <button className="button tertiary icon-only" type="button" onClick={resetToSample} aria-label="Reset sample">
              <RotateCcw size={18} />
            </button>
          </div>
        </section>

        <section className="comparison" aria-label="Before and after">
          <article className="image-card">
            <div className="card-heading">
              <div>
                <span className="kicker">Before</span>
                <h2>Ảnh gốc</h2>
              </div>
              <span className="file-pill">{selectedName}</span>
            </div>
            <div className="image-frame">
              <img src={beforeUrl} alt="Original ring" />
            </div>
          </article>

          <article className="image-card after-card">
            <div className="card-heading">
              <div>
                <span className="kicker">After</span>
                <h2>Studio gold</h2>
              </div>
              {result ? (
                <span className="file-pill ready">
                  <CheckCircle2 size={14} />
                  {result.model}
                </span>
              ) : (
                <span className="file-pill">Ready</span>
              )}
            </div>

            <div className="image-frame">
              {afterUrl ? (
                <img src={afterUrl} alt="Enhanced gold ring" />
              ) : (
                <div className="empty-state">
                  {isLoading ? <LoaderCircle className="spin" size={34} /> : <Sparkles size={34} />}
                  <strong>{isLoading ? 'Đang render ánh sáng studio' : 'Ảnh sau xử lý'}</strong>
                  <span>{isLoading ? 'Giữ nguyên chi tiết nhẫn' : 'Kết quả GPT Image 2 sẽ xuất hiện ở đây'}</span>
                </div>
              )}
            </div>

            <div className="result-bar">
              <span>{result?.quality ?? quality}</span>
              <span>{result?.size ?? size}</span>
              <span>{result?.outputFormat ?? outputFormat}</span>
              <button className="button tertiary compact" type="button" disabled={!afterUrl} onClick={downloadResult}>
                <Download size={16} />
                Tải ảnh
              </button>
            </div>
          </article>
        </section>
      </main>
    </div>
  )
}

export default App
