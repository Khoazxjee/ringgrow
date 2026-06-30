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
  Upload,
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

type BatchResultItem = {
  name: string
  input: string
  output?: string
  status: 'done' | 'failed'
  error?: string
}

type BatchResult = {
  total: number
  succeeded: number
  failed: number
  outputDir: string
  items: BatchResultItem[]
}

type Quality = 'high' | 'medium' | 'auto'
type Size = 'auto' | '1536x1024' | '1024x1024' | '1024x1536'
type OutputFormat = 'png' | 'jpeg' | 'webp'

const API_BASE = (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? '/api'
const GOLD_REFERENCE = '/examples/gold-reference.jpg'

const qualityOptions: Array<{ label: string; value: Quality }> = [
  { label: 'Cao', value: 'high' },
  { label: 'Cân bằng', value: 'medium' },
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
  { icon: ShieldCheck, title: 'Giữ nguyên chi tiết', text: 'Hoa văn, đá và dáng nhẫn được bảo toàn' },
  { icon: Gem, title: 'Đồng nhất màu vàng', text: 'Bám theo mẫu màu chuẩn' },
  { icon: Camera, title: 'Ảnh sạch studio', text: 'Cạnh rõ, ánh sáng tự nhiên' },
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
  const [batchSource, setBatchSource] = useState('')
  const [batchOutputDir, setBatchOutputDir] = useState('')
  const [isBatchRunning, setIsBatchRunning] = useState(false)
  const [batchError, setBatchError] = useState('')
  const [batchResult, setBatchResult] = useState<BatchResult | null>(null)

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
  const batchEndpoint = `${API_BASE.replace(/\/$/, '')}/batch`
  const canGenerate = !isLoading && Boolean(file && beforeUrl)
  const canReset = Boolean(file || beforeUrl || afterUrl || result || error)
  const canRunBatch = !isBatchRunning && Boolean(batchSource.trim() && batchOutputDir.trim())

  function chooseFile(nextFile: File) {
    if (!nextFile.type.startsWith('image/')) {
      setError('Vui lòng chọn ảnh JPG, PNG hoặc WEBP.')
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
      setError('Vui lòng tải ảnh gốc lên trước khi chuyển đổi.')
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
        throw new Error(payload.error ?? 'Chưa xử lý được ảnh. Vui lòng thử lại.')
      }

      setAfterUrl(payload.image)
      setResult(payload)
    } catch (caught) {
      setAfterUrl('')
      setError(caught instanceof Error ? caught.message : 'Chưa xử lý được ảnh. Vui lòng thử lại.')
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

  async function runBatch() {
    if (!batchSource.trim() || !batchOutputDir.trim()) {
      setBatchError('Vui lòng nhập nguồn ảnh và folder xuất kết quả.')
      return
    }

    setIsBatchRunning(true)
    setBatchError('')
    setBatchResult(null)

    try {
      const response = await fetch(batchEndpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          source: batchSource.trim(),
          outputDir: batchOutputDir.trim(),
          quality,
          size,
        }),
      })
      const payload = await response.json()

      if (!response.ok) {
        throw new Error(payload.error ?? 'Chưa xử lý được album. Vui lòng kiểm tra lại đường dẫn.')
      }

      setBatchResult(payload)
    } catch (caught) {
      setBatchError(caught instanceof Error ? caught.message : 'Chưa xử lý được album. Vui lòng thử lại.')
    } finally {
      setIsBatchRunning(false)
    }
  }

  return (
    <div className="app-shell">
      <header className="top-region">
        <nav className="top-nav" aria-label="Điều hướng chính">
          <div className="brand">
            <span className="brand-mark" aria-hidden="true" />
            <span>RingGlow</span>
          </div>
          <div className="nav-actions" aria-label="Trạng thái xử lý">
            <span className="status-pill muted">Chuẩn màu studio</span>
            <span className="status-pill">gpt-image-2</span>
          </div>
        </nav>
      </header>

      <main className="studio-page">
        <section className="studio-hero" aria-labelledby="studio-title">
          <div>
            <span className="eyebrow">Studio ảnh nhẫn AI</span>
            <h1 id="studio-title">Chuẩn màu vàng studio, giữ trọn chi tiết nhẫn.</h1>
            <p>Tải ảnh nhẫn gốc lên, RingGlow sẽ cân màu vàng tự nhiên hơn, làm sạch cạnh và giữ nguyên hoa văn, đá, bố cục.</p>
          </div>
          <div className="highlight-row" aria-label="Điểm nổi bật">
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
          <aside className="control-panel" aria-label="Thiết lập ảnh">
            <div className="panel-title">
              <Settings2 size={18} strokeWidth={1.8} />
              <div>
                <h2>Thiết lập ảnh xuất</h2>
                <p>Chọn chất lượng, khung ảnh và định dạng trước khi chuyển đổi.</p>
              </div>
            </div>

            <div className="settings-stack">
              <ControlGroup
                title="Chất lượng"
                note="Cao cho ảnh bán hàng"
                ariaLabel="Chất lượng ảnh"
                options={qualityOptions}
                value={quality}
                onChange={setQuality}
              />
              <ControlGroup
                title="Khung ảnh"
                note="Tự động theo ảnh gốc"
                ariaLabel="Khung ảnh"
                options={sizeOptions}
                value={size}
                onChange={setSize}
              />
              <ControlGroup
                title="Định dạng"
                note="PNG sắc nét nhất"
                ariaLabel="Định dạng xuất"
                options={formatOptions}
                value={outputFormat}
                onChange={setOutputFormat}
              />
            </div>

            <div className="reference-card">
              <div>
                <span>Màu chuẩn</span>
                <strong>Vàng studio tự nhiên</strong>
                <small>Áp dụng đồng nhất cho mọi ảnh</small>
              </div>
              <img src={GOLD_REFERENCE} alt="Mẫu màu vàng studio" />
            </div>
          </aside>

          <section className="preview-area" aria-label="So sánh ảnh gốc và kết quả">
            <div className="preview-heading">
              <div>
                <span className="eyebrow">Xem trước</span>
                <h2>Ảnh gốc và kết quả</h2>
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

            <div className="comparison">
              <ImageCard label="Gốc" title="Ảnh gốc" badge={beforeUrl ? selectedName : 'Chưa chọn'}>
                {beforeUrl ? (
                  <>
                    <img src={beforeUrl} alt="Ảnh nhẫn gốc" />
                    <button className="change-image-button" type="button" onClick={() => fileInputRef.current?.click()}>
                      Đổi ảnh
                    </button>
                  </>
                ) : (
                  <button
                    className={`image-upload-dropzone ${isDragging ? 'is-dragging' : ''}`}
                    type="button"
                    onClick={() => fileInputRef.current?.click()}
                    onDragOver={(event) => {
                      event.preventDefault()
                      setIsDragging(true)
                    }}
                    onDragLeave={() => setIsDragging(false)}
                    onDrop={onDrop}
                  >
                    <span className="image-upload-icon" aria-hidden="true">
                      <Upload size={28} strokeWidth={1.8} />
                    </span>
                    <span>Tải ảnh gốc lên</span>
                  </button>
                )}
              </ImageCard>

              <ImageCard
                className="after-card"
                label="Kết quả"
                title="Vàng studio"
                badge={
                  result ? (
                    <>
                      <CheckCircle2 size={14} />
                      {result.model}
                    </>
                  ) : (
                    'Chờ xử lý'
                  )
                }
              >
                {afterUrl ? (
                  <img src={afterUrl} alt="Ảnh nhẫn sau khi chuẩn màu vàng" />
                ) : (
                  <div className="empty-state">
                    <span className="empty-icon" aria-hidden="true">
                      {isLoading ? <LoaderCircle className="spin" size={32} /> : <Sparkles size={32} />}
                    </span>
                    <strong>{isLoading ? 'Đang xử lý ảnh' : 'Kết quả sẽ hiện tại đây'}</strong>
                    <span>{isLoading ? 'RingGlow đang cân màu và giữ nguyên chi tiết nhẫn.' : 'Bấm Chuyển đổi ảnh để tạo bản vàng studio.'}</span>
                  </div>
                )}
              </ImageCard>
            </div>

            {beforeUrl ? (
                <div className="download-strip">
                  <div>
                    <strong>{afterUrl ? 'Kết quả đã sẵn sàng' : 'Chưa có kết quả'}</strong>
                    <span>{afterUrl ? 'Bạn có thể tải ảnh đã xử lý về máy.' : 'Kết quả sẽ dùng thiết lập hiện tại.'}</span>
                  </div>
                  <button className="button tertiary compact" type="button" disabled={!afterUrl} onClick={downloadResult}>
                    <Download size={16} />
                    Tải kết quả
                  </button>
                </div>
            ) : null}
          </section>

          {error ? <div className="error-message">{error}</div> : null}

          <div className="conversion-footer">
            <div>
              <strong>{beforeUrl ? 'Sẵn sàng chuyển đổi' : 'Tải ảnh gốc lên để bắt đầu'}</strong>
              <span>
                {beforeUrl
                  ? 'RingGlow sẽ cân màu vàng, làm sạch cạnh và giữ nguyên chi tiết nhẫn.'
                  : 'Chọn ảnh nhẫn ở khung bên trên, sau đó bấm Chuyển đổi ảnh.'}
              </span>
            </div>
            <div className="conversion-actions">
              <button className="button primary convert-button" type="button" disabled={!canGenerate} onClick={enhanceImage}>
                {isLoading ? <LoaderCircle className="spin" size={18} /> : <WandSparkles size={18} />}
                {isLoading ? 'Đang chuyển đổi' : afterUrl ? 'Chuyển đổi lại' : 'Chuyển đổi ảnh'}
              </button>
              <button className="button tertiary icon-only" type="button" disabled={!canReset} onClick={resetImage} aria-label="Xóa ảnh">
                <RotateCcw size={18} />
              </button>
            </div>
          </div>

          <section className="batch-panel" aria-labelledby="batch-title">
            <div className="batch-heading">
              <div>
                <span className="eyebrow">Xử lý album</span>
                <h2 id="batch-title">Chạy hàng loạt từ folder hoặc link Drive</h2>
                <p>Lưu kết quả sang folder bạn chỉ định, giữ nguyên tên file và phần mở rộng của ảnh gốc.</p>
              </div>
            </div>

            <div className="batch-form">
              <label className="batch-field">
                <span>Folder ảnh hoặc link Drive</span>
                <input
                  type="text"
                  value={batchSource}
                  onChange={(event) => setBatchSource(event.currentTarget.value)}
                  placeholder="Ví dụ: C:\\Users\\Bạn\\Pictures\\nhan-goc hoặc link folder Drive công khai"
                />
                <small>Hỗ trợ folder local, link folder/file ảnh Drive công khai hoặc file ZIP chứa nhiều ảnh.</small>
              </label>

              <label className="batch-field">
                <span>Folder xuất kết quả</span>
                <input
                  type="text"
                  value={batchOutputDir}
                  onChange={(event) => setBatchOutputDir(event.currentTarget.value)}
                  placeholder="Ví dụ: C:\\Users\\Bạn\\Pictures\\nhan-da-xu-ly"
                />
                <small>Folder sẽ được tạo nếu chưa tồn tại.</small>
              </label>
            </div>

            {batchError ? <div className="error-message">{batchError}</div> : null}

            <div className="batch-footer">
              <div>
                <strong>{batchResult ? `Đã xử lý ${batchResult.succeeded}/${batchResult.total} ảnh` : 'Dùng thiết lập hiện tại cho cả album'}</strong>
                <span>
                  {batchResult
                    ? `Kết quả được lưu tại ${batchResult.outputDir}.`
                    : 'Mỗi ảnh sẽ được chuẩn màu vàng studio và xuất ra đúng tên, đúng định dạng gốc.'}
                </span>
              </div>
              <button className="button primary batch-button" type="button" disabled={!canRunBatch} onClick={runBatch}>
                {isBatchRunning ? <LoaderCircle className="spin" size={18} /> : <WandSparkles size={18} />}
                {isBatchRunning ? 'Đang xử lý album' : 'Chạy album'}
              </button>
            </div>

            {batchResult ? (
              <div className="batch-result">
                <div className="batch-summary">
                  <span>Thành công: {batchResult.succeeded}</span>
                  <span>Lỗi: {batchResult.failed}</span>
                </div>
                <div className="batch-list">
                  {batchResult.items.slice(0, 8).map((item) => (
                    <div className={`batch-row ${item.status}`} key={`${item.name}-${item.input}`}>
                      <span>{item.name}</span>
                      <small>{item.status === 'done' ? 'Đã lưu' : item.error}</small>
                    </div>
                  ))}
                  {batchResult.items.length > 8 ? <small className="batch-more">Còn {batchResult.items.length - 8} ảnh khác trong kết quả.</small> : null}
                </div>
              </div>
            ) : null}
          </section>
        </div>
      </main>

      {downloadToast ? (
        <div className="download-toast" role="status" aria-live="polite">
          <CheckCircle2 size={18} />
          <div>
            <strong>Đã tải kết quả</strong>
            <span>Ảnh đã được lưu về máy.</span>
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
