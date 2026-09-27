import themes from './index'
import { describe, it, expect } from 'vitest'

describe('NDPlaylistDetails styles', () => {
  const themeEntries = Object.entries(themes)

  it.each(themeEntries)(
    '%s should not set minWidth on details',
    (themeName, theme) => {
      const details = theme.overrides?.NDPlaylistDetails?.details
      expect(details?.minWidth).toBeUndefined()
    },
  )
})

describe('NDAlbumGridView styles', () => {
  const themeEntries = Object.entries(themes)

  // The hover overlay is a sibling of the image, so it keeps square corners.
  it.each(themeEntries)(
    '%s should not round the grid cover image on its own',
    (themeName, theme) => {
      const container = theme.overrides?.NDAlbumGridView?.albumContainer
      expect(container?.['& img']?.borderRadius).toBeUndefined()
    },
  )

  it.each(themeEntries)(
    '%s should clip the grid cover link when it is rounded',
    (themeName, theme) => {
      const link = theme.overrides?.NDAlbumGridView?.link
      if (!link?.borderRadius) return
      expect(link.overflow).toBe('hidden')
    },
  )
})

describe('playback dock theming', () => {
  const themeEntries = Object.entries(themes)

  // 播放 dock 的配色走 CSS 变量（由 useCurrentTheme 写进 <html>），
  // 因为 JSS 会按首次 theme 缓存规则、切换主题后不重算。
  // 这里守住前提：每个主题都必须声明 palette.type，否则深浅判断会失效。
  it.each(themeEntries)(
    '%s should declare palette.type so the dock can tell light from dark',
    (themeName, theme) => {
      expect(['light', 'dark']).toContain(theme.palette?.type)
    },
  )
})
