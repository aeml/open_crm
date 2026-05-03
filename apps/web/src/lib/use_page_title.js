import { useEffect } from 'react'

export function usePageTitle(title) {
  useEffect(() => {
    document.title = title ? `${title} — Open CRM` : 'Open CRM'
    return () => {
      document.title = 'Open CRM'
    }
  }, [title])
}
