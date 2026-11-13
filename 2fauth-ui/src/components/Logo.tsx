import logoUrl from '../assets/logo.svg'

/** The 2FAuth mark: two ribbons around a star. */
export default function Logo({ size, className = '' }: { size?: number; className?: string }) {
  return (
    <img
      src={logoUrl}
      alt=""
      aria-hidden="true"
      draggable={false}
      className={`brand-logo ${className}`.trim()}
      style={size ? { width: size, height: size } : undefined}
    />
  )
}
