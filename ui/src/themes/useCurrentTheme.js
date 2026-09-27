import { useSelector } from 'react-redux'
import useMediaQuery from '@material-ui/core/useMediaQuery'
import themes from './index'
import { AUTO_THEME_ID } from '../consts'
import config from '../config'
import { useEffect, useMemo } from 'react'

const useCurrentTheme = () => {
  // Runs above the ThemeProvider carrying the prop below, so it needs its own noSsr or the
  // auto theme renders dark first and flips.
  const prefersLightMode = useMediaQuery('(prefers-color-scheme: light)', {
    noSsr: true,
  })
  const theme = useSelector((state) => {
    if (state.theme === AUTO_THEME_ID) {
      return prefersLightMode ? themes.LightTheme : themes.DarkTheme
    }
    const themeName =
      Object.keys(themes).find((t) => t === state.theme) ||
      Object.keys(themes).find(
        (t) => themes[t].themeName === config.defaultTheme,
      ) ||
      'DarkTheme'
    return themes[themeName]
  })

  useEffect(() => {
    const styles = document.getElementsByTagName('style')
    let style
    for (let i = 0; i < styles.length; i++) {
      if (styles[i].id === 'nd-player-style-override') {
        style = styles[i]
      }
    }
    if (theme.player.stylesheet) {
      if (style === undefined) {
        style = document.createElement('style')
        style.id = 'nd-player-style-override'
        style.innerHTML = theme.player.stylesheet
        document.head.appendChild(style)
      } else {
        style.innerHTML = theme.player.stylesheet
      }
    } else {
      if (style !== undefined) {
        document.head.removeChild(style)
      }
    }

    // Set body background color to match theme (fixes white background on pull-to-refresh)
    const isDark = theme.palette?.type === 'dark'
    const bgColor =
      theme.palette?.background?.default || (isDark ? '#303030' : '#fafafa')
    document.body.style.backgroundColor = bgColor

    // 播放 dock 的配色变量。写在这里而不是 styles.js 里按 isDark 硬编码：
    // JSS 会按首次 theme 缓存规则，切换主题后不重算，dock 配色会停在旧主题。
    // 变量由 CSS 直接读取，主题一变立刻生效。
    const root = document.documentElement
    if (isDark) {
      root.style.setProperty('--nd-dock-bg', 'rgba(20, 24, 36, 0.88)')
      root.style.setProperty('--nd-dock-border', 'rgba(255, 255, 255, 0.08)')
      root.style.setProperty(
        '--nd-dock-shadow',
        '0 -4px 24px rgba(0, 0, 0, 0.45)',
      )
    } else {
      root.style.setProperty('--nd-dock-bg', 'rgba(255, 255, 255, 0.92)')
      root.style.setProperty('--nd-dock-border', 'rgba(0, 0, 0, 0.08)')
      root.style.setProperty(
        '--nd-dock-shadow',
        '0 -4px 20px rgba(0, 0, 0, 0.08)',
      )
    }
  }, [theme])

  // We never server-render, so let media queries resolve on the first render: the default
  // defers them to an effect, which makes every mount paint the wrong breakpoint and reflow.
  return useMemo(
    () => ({
      ...theme,
      props: {
        ...theme.props,
        MuiUseMediaQuery: { noSsr: true },
        MuiPopover: { disableScrollLock: true },
      },
    }),
    [theme],
  )
}

export default useCurrentTheme
