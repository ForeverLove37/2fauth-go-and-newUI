interface IconProps {
  name: IconName
  size?: number
}

export type IconName =
  | 'copy'
  | 'star'
  | 'star-filled'
  | 'sliders'
  | 'share'
  | 'qr'
  | 'trash'
  | 'close'
  | 'plus'
  | 'search'
  | 'refresh'
  | 'lock'
  | 'shield'
  | 'logout'
  | 'eye'
  | 'eye-off'
  | 'grid'
  | 'orbit'
  | 'upload'
  | 'download'
  | 'users'
  | 'check'

const paths: Record<IconName, string> = {
  copy: 'M9 9V5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-4M5 9h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-8a2 2 0 0 1 2-2Z',
  star: 'M12 3.6l2.6 5.3 5.9.9-4.3 4.1 1 5.8-5.2-2.7-5.2 2.7 1-5.8L3.5 9.8l5.9-.9L12 3.6Z',
  'star-filled': 'M12 3.6l2.6 5.3 5.9.9-4.3 4.1 1 5.8-5.2-2.7-5.2 2.7 1-5.8L3.5 9.8l5.9-.9L12 3.6Z',
  sliders: 'M4 6h10M18 6h2M4 12h4M12 12h8M4 18h12M20 18h0M14 4v4M8 10v4M16 16v4',
  share: 'M8.6 13.5l6.8 3.9M15.4 6.6L8.6 10.5M18 8.2a2.6 2.6 0 1 1 0-5.2 2.6 2.6 0 0 1 0 5.2ZM6 14.6a2.6 2.6 0 1 1 0-5.2 2.6 2.6 0 0 1 0 5.2ZM18 21.2a2.6 2.6 0 1 1 0-5.2 2.6 2.6 0 0 1 0 5.2Z',
  qr: 'M4 4h6v6H4V4Zm10 0h6v6h-6V4ZM4 14h6v6H4v-6Zm10 3h3v3h-3v-3Zm3-3h3v3h-3v-3Zm-3-3h3v3h-3v-3Z',
  trash: 'M4 7h16M9 7V5a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2M6 7l1 13a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1l1-13M10 11v6M14 11v6',
  close: 'M6 6l12 12M18 6L6 18',
  plus: 'M12 5v14M5 12h14',
  search: 'M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14Zm5.2-1.8L21 21',
  refresh: 'M20 12a8 8 0 1 1-2.3-5.7M20 4v4h-4',
  lock: 'M6 11h12a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1v-7a1 1 0 0 1 1-1Zm2 0V8a4 4 0 0 1 8 0v3',
  shield: 'M12 3l8 3v6c0 4.4-3.4 8.2-8 9-4.6-.8-8-4.6-8-9V6l8-3Zm-3 9l2.2 2.2L15.5 10',
  logout: 'M15 12H4m4-4l-4 4 4 4M14 4h4a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-4',
  eye: 'M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12Zm9.5 2.6a2.6 2.6 0 1 0 0-5.2 2.6 2.6 0 0 0 0 5.2Z',
  'eye-off': 'M4 4l16 16M10 6a9.6 9.6 0 0 1 2-.2c6 0 9.5 6.2 9.5 6.2a17 17 0 0 1-3.3 4M6.7 7.8A16.9 16.9 0 0 0 2.5 12S6 18.2 12 18.2c1.3 0 2.5-.3 3.6-.8',
  grid: 'M4 4h7v7H4V4Zm9 0h7v7h-7V4ZM4 13h7v7H4v-7Zm9 0h7v7h-7v-7Z',
  orbit: 'M12 12a3 3 0 1 0 0 6 3 3 0 0 0 0-6Zm0 0c3.9-2.3 7.4-2.6 8.4-.9 1 1.7-1.2 4.4-4.9 6.6M12 12c-3.9 2.3-7.4 2.6-8.4.9-1-1.7 1.2-4.4 4.9-6.6',
  upload: 'M12 17V5m0 0L8 9m4-4 4 4M4 17v2a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-2',
  download: 'M12 5v12m0 0 4-4m-4 4-4-4M4 19h16',
  users: 'M8 11a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Zm8 0a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7ZM2 20c0-3 2.7-5 6-5s6 2 6 5M16 15c3 0 6 2 6 5',
  check: 'M5 13l4 4L19 7',
}

/** Tiny stroke icon set, no icon dependency. */
export default function Icon({ name, size = 16 }: IconProps) {
  const filled = name === 'star-filled'
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill={filled ? 'currentColor' : 'none'}
      stroke="currentColor"
      strokeWidth={filled ? 1 : 1.7}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d={paths[name]} />
    </svg>
  )
}
