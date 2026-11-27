import { useEffect, useMemo, useRef } from 'preact/hooks'
import { encode } from 'uqr'

/** A QR code as SVG. Always black on white with a quiet zone, so it scans in the dark theme too. */
export function QR({ text, label }: { text: string; label?: string }) {
  const { size, path } = useMemo(() => {
    const qr = encode(text, { ecc: 'M', border: 3 })
    let d = ''
    qr.data.forEach((row, y) => row.forEach((on, x) => on && (d += `M${x} ${y}h1v1h-1z`)))
    return { size: qr.size, path: d }
  }, [text])
  return (
    <svg class="qr" viewBox={`0 0 ${size} ${size}`} role={label ? 'img' : undefined} aria-label={label} aria-hidden={!label} shape-rendering="crispEdges">
      <rect width={size} height={size} fill="#fff" />
      <path d={path} fill="#000" />
    </svg>
  )
}

/** The join QR code large, for the projector. */
export function JoinQRDialog(p: { link: string; joinUrl: string; code: string; onClose: () => void }) {
  const dlg = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    dlg.current?.showModal()
  }, [])
  return (
    <dialog
      ref={dlg}
      class="task-dialog qr-dialog"
      aria-labelledby="qr-title"
      onCancel={(e) => (e.preventDefault(), p.onClose())}
      onClick={(e) => e.target === dlg.current && p.onClose()}
    >
      <header class="task-dialog-head">
        <h2 id="qr-title">Team gründen</h2>
        <span class="spacer" />
        <button class="btn btn-quiet btn-small" onClick={p.onClose}>
          Schließen
        </button>
      </header>
      <div class="qr-body">
        <QR text={p.link} label={`QR-Code: ${p.link}`} />
        <div class="qr-text">
          <p>Scannt den QR-Code. Der Lab-Code ist dann schon eingetragen.</p>
          <p>
            Oder öffnet <strong>{p.joinUrl}</strong> und gebt den Code ein:
          </p>
          <span class="labcode-value">{p.code}</span>
        </div>
      </div>
    </dialog>
  )
}
