import type { ReactNode } from 'react'

export function PageHead({
  index,
  title,
  meta,
  right,
}: {
  index: string
  title: string
  meta?: ReactNode
  right?: ReactNode
}) {
  return (
    <div className="sec-head">
      <span className="sec-head__index">{index}</span>
      <h1 className="sec-head__title">{title}</h1>
      <span className="sec-head__rule" />
      <span className="sec-head__meta">{meta}</span>
      {right}
    </div>
  )
}