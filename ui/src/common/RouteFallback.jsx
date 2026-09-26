import React from 'react'
import CircularProgress from '@material-ui/core/CircularProgress'
import { useTranslate } from 'react-admin'

// Suspense fallback for lazily loaded routes (react-refresh requires a module
// to export components only, so it lives in its own file).
const RouteFallback = () => {
  const translate = useTranslate()
  return (
    <div
      style={{
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        padding: 48,
      }}
    >
      <CircularProgress size={24} aria-label={translate('ra.page.loading')} />
    </div>
  )
}

export default RouteFallback
