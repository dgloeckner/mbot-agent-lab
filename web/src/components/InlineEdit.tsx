import { useEffect, useRef, useState } from 'preact/hooks'

/**
 * Text that turns into an input on click. Enter saves, Escape cancels.
 * onSave returns whether saving worked; on failure the input stays open.
 */
export function InlineEdit(props: {
  value: string
  label: string
  maxLength: number
  class?: string
  onSave: (value: string) => Promise<boolean>
}) {
  const [editing, setEditing] = useState(false)
  const [text, setText] = useState(props.value)
  const [saving, setSaving] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  // Enter saves and disables the input, which then loses the focus and would
  // save a second time on blur: one save per edit.
  const inFlight = useRef(false)

  // Select the whole name when editing starts, so typing replaces it.
  useEffect(() => {
    if (editing) {
      input.current?.focus()
      input.current?.select()
    }
  }, [editing])

  if (!editing) {
    return (
      <button
        class={`inline-edit ${props.class ?? ''}`}
        title={`${props.label} ändern`}
        aria-label={`${props.value}. ${props.label} ändern`}
        onClick={() => {
          setText(props.value)
          setEditing(true)
        }}
      >
        {props.value}
      </button>
    )
  }
  const save = async () => {
    if (inFlight.current) return
    const v = text.trim()
    if (!v || v === props.value) return setEditing(false)
    inFlight.current = true
    setSaving(true)
    try {
      const ok = await props.onSave(v)
      if (ok) setEditing(false)
    } finally {
      inFlight.current = false
      setSaving(false)
    }
  }
  return (
    <input
      class={`inline-input ${props.class ?? ''}`}
      value={text}
      maxLength={props.maxLength}
      aria-label={props.label}
      disabled={saving}
      size={Math.max(4, text.length + 1)}
      ref={input}
      onInput={(e) => setText(e.currentTarget.value)}
      onBlur={() => void save()}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          e.preventDefault()
          void save()
        } else if (e.key === 'Escape') {
          setEditing(false)
        }
      }}
    />
  )
}
