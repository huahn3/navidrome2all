import React, { useState, useCallback } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { useTranslate, useNotify } from 'react-admin'
import IconButton from '@material-ui/core/IconButton'
import Tooltip from '@material-ui/core/Tooltip'
import CircularProgress from '@material-ui/core/CircularProgress'
import TranslateIcon from '@material-ui/icons/Translate'
import { makeStyles } from '@material-ui/core/styles'
import httpClient from '../dataProvider/httpClient'
import { updateSongLyric } from '../actions'

const useStyles = makeStyles((theme) => ({
  active: {
    color: `${theme.palette.secondary.main} !important`,
  },
  progress: {
    color: theme.palette.text.secondary,
  },
}))

const TranslateButton = ({
  id,
  isRadio,
  isDesktop,
  buttonClassName,
  iconClassName,
}) => {
  const dispatch = useDispatch()
  const translate = useTranslate()
  const notify = useNotify()
  const classes = useStyles()

  const playerState = useSelector((state) => state.player || {})
  const current = playerState.current || {}
  const isBilingual = !!playerState.bilingualActive
  const cachedBilingual = playerState.bilingualLyrics?.[id]
  const cachedOriginal =
    playerState.originalLyrics?.[id] ||
    (current.trackId === id ? current.lyric : '')

  const [translating, setTranslating] = useState(false)

  const handleTranslate = useCallback(
    async (e) => {
      e.stopPropagation()
      if (!id || isRadio || translating) {
        return
      }

      if (isBilingual) {
        // Toggle back to original lyrics
        const orig = cachedOriginal || current.lyric
        if (orig) {
          dispatch(updateSongLyric(id, orig, false))
        }
        notify(
          translate('player.showOriginalLyricSuccess', {
            _: '已恢复原文歌词',
          }),
          { type: 'info', autoHideDuration: 2500 },
        )
        return
      }

      // Check if already in memory
      if (cachedBilingual) {
        dispatch(updateSongLyric(id, cachedBilingual, true))
        notify(
          translate('player.showBilingualLyricSuccess', {
            _: '已切换为双语对照歌词',
          }),
          { type: 'success', autoHideDuration: 2500 },
        )
        return
      }

      // If not yet translated: prompt user and fetch
      notify(translate('player.translating', { _: '正在翻译歌词...' }), {
        type: 'info',
        autoHideDuration: 2000,
      })
      setTranslating(true)

      try {
        // Add 35-second client-side timeout to avoid endless spinning
        const timeoutPromise = new Promise((_, reject) =>
          setTimeout(
            () =>
              reject(new Error('翻译请求超时，请检查网络、代理或API密钥设置')),
            35000,
          ),
        )

        // Backend automatically resolves configured targetLanguage, engine, and API keys
        const fetchPromise = httpClient('/api/lyrics/translate', {
          method: 'POST',
          body: JSON.stringify({
            songId: id,
          }),
        })

        const res = await Promise.race([fetchPromise, timeoutPromise])

        const bilingualLrc =
          res.json?.inlineLrc ||
          res.json?.combinedLrc ||
          res.json?.bilingualLrc ||
          ''
        if (!bilingualLrc) {
          notify(
            translate('player.translateEmpty', { _: '未找到可翻译的歌词' }),
            {
              type: 'warning',
            },
          )
          return
        }

        dispatch(updateSongLyric(id, bilingualLrc, true))
        notify(
          translate('player.translateSuccess', {
            _: '歌词翻译完成，已显示双语对照',
          }),
          { type: 'success', autoHideDuration: 2500 },
        )
      } catch (err) {
        const status = err?.status
        const detail =
          typeof err?.body === 'string'
            ? err.body
            : err?.body?.error || err?.message || ''

        if (status === 404) {
          notify(
            translate('player.translateNotFound', {
              _: '当前歌曲暂无歌词可供翻译',
            }),
            { type: 'warning' },
          )
        } else if (status === 403) {
          notify(
            translate('player.translateForbidden', {
              _: '歌词翻译功能未开启，请在「歌词翻译」独立设置中开启',
            }),
            { type: 'error' },
          )
        } else {
          notify(
            translate('player.translateFailed', {
              _: detail
                ? `歌词翻译失败: ${detail}`
                : '歌词翻译失败，请检查网络或API配置',
            }),
            { type: 'error' },
          )
        }
      } finally {
        setTranslating(false)
      }
    },
    [
      id,
      isRadio,
      translating,
      isBilingual,
      cachedBilingual,
      cachedOriginal,
      current.lyric,
      dispatch,
      notify,
      translate,
    ],
  )

  const handleForceRetranslate = useCallback(
    async (e) => {
      e.preventDefault()
      e.stopPropagation()
      if (!id || isRadio || translating) {
        return
      }

      notify(
        translate('player.retranslating', {
          _: '正在使用最新模型重新翻译当前歌曲...',
        }),
        { type: 'info', autoHideDuration: 2500 },
      )
      setTranslating(true)

      try {
        const res = await httpClient('/api/lyrics/translate', {
          method: 'POST',
          body: JSON.stringify({
            songId: id,
            force: true,
          }),
        })

        const bilingualLrc =
          res.json?.inlineLrc ||
          res.json?.combinedLrc ||
          res.json?.bilingualLrc ||
          ''
        if (!bilingualLrc) {
          notify(
            translate('player.translateEmpty', { _: '未找到可翻译的歌词' }),
            { type: 'warning' },
          )
          return
        }

        dispatch(updateSongLyric(id, bilingualLrc, true))
        notify(
          translate('player.retranslateSuccess', {
            _: '已使用最新模型重新翻译并展示双语歌词！',
          }),
          { type: 'success', autoHideDuration: 3000 },
        )
      } catch (err) {
        notify(
          translate('player.retranslateFailed', {
            _: `重新翻译失败: ${err?.message || '请检查模型或网络配置'}`,
          }),
          { type: 'error' },
        )
      } finally {
        setTranslating(false)
      }
    },
    [id, isRadio, translating, dispatch, notify, translate],
  )

  const tooltipTitle = translating
    ? translate('player.translating', { _: '正在翻译歌词...' })
    : isBilingual
      ? translate('player.showOriginalLyric', {
          _: '显示原文歌词 (右键单击使用最新模型重译)',
        })
      : translate('player.translateLyric', {
          _: '翻译歌词 (双语对照，右键单击强制重译)',
        })

  return (
    <Tooltip title={tooltipTitle} enterDelay={300}>
      <span>
        <IconButton
          size={isDesktop ? 'small' : undefined}
          disableRipple={!isDesktop}
          onClick={handleTranslate}
          onContextMenu={handleForceRetranslate}
          disabled={isRadio || !id}
          data-testid="translate-lyrics-button"
          className={`${buttonClassName} ${isBilingual ? classes.active : ''}`}
          aria-label={tooltipTitle}
        >
          {translating ? (
            <CircularProgress size={18} className={classes.progress} />
          ) : (
            <TranslateIcon className={iconClassName} />
          )}
        </IconButton>
      </span>
    </Tooltip>
  )
}

export default TranslateButton
