import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useInterval } from '../common'
import { useDispatch, useSelector } from 'react-redux'
import { ThemeProvider } from '@material-ui/core/styles'
import {
  createMuiTheme,
  useAuthState,
  useDataProvider,
  useNotify,
  useTranslate,
} from 'react-admin'
import ReactGA from 'react-ga'
import { GlobalHotKeys } from 'react-hotkeys'
import ReactJkMusicPlayer from 'navidrome-music-player'
import 'navidrome-music-player/assets/index.css'
import useCurrentTheme from '../themes/useCurrentTheme'
import config from '../config'
import useStyle from './styles'
import AudioTitle from './AudioTitle'
import {
  BROWSER_DEVICE,
  clearPendingSeek,
  clearQueue,
  currentPlaying,
  refreshQueue,
  setPlayMode,
  setTranscodingProfile,
  setVolume,
  syncQueue,
  updateSongLyric,
} from '../actions'
import PlayerToolbar from './PlayerToolbar'
import * as jukebox from './jukebox'
import { sendNotification } from '../utils'
import httpClient, { clientUniqueId } from '../dataProvider/httpClient'
import subsonic from '../subsonic'
import locale from './locale'
import { keyMap } from '../hotkeys'
import keyHandlers from './keyHandlers'
import { calculateGain } from '../utils/calculateReplayGain'
import { detectBrowserProfile, decisionService } from '../transcode'

const Player = () => {
  const theme = useCurrentTheme()
  const translate = useTranslate()
  const notify = useNotify()
  const playerTheme = theme.player?.theme || 'dark'
  const dataProvider = useDataProvider()
  const playerState = useSelector((state) => state.player)
  const dispatch = useDispatch()
  const [currentTrackId, setCurrentTrackId] = useState(null)
  const [heartbeatTrackId, setHeartbeatTrackId] = useState(null)
  const lastPositionMsRef = useRef(0)
  const currentTrackIdRef = useRef(null)
  const stoppedRef = useRef(false)
  const playerRef = useRef(null)
  const [audioInstance, setAudioInstance] = useState(null)
  // Multi-output (jukebox) support
  const outputDevice = playerState.outputDevice || BROWSER_DEVICE
  const outputDeviceRef = useRef(outputDevice)
  const remoteActive = outputDevice !== BROWSER_DEVICE
  const remoteActiveRef = useRef(remoteActive)
  const remoteTrackRef = useRef(null) // track currently driven on the remote output
  const remotePlayingRef = useRef(false) // remote confirmed playing the current track
  const suppressSeekRef = useRef(false) // marks drift corrections (do not echo them back)
  const remoteNoProgressRef = useRef(false) // remote cannot report position (e.g. xiaomi)
  const lastUserPlayRef = useRef(0)
  const lastUserPauseRef = useRef(0)
  const lastRemoteErrorRef = useRef(0)
  const isHandoffPausedRef = useRef(false) // true when taken over by another device
  // Status polling degrades to a slow retry loop while a remote output keeps
  // failing, so an unreachable device does not flood the log every second
  const [remoteStatusDelay, setRemoteStatusDelay] = useState(1000)
  const remoteStatusFailuresRef = useRef(0)
  const remoteStatusErrorShownRef = useRef(false)
  // Volume: the store holds the single source of truth (0..1, perceptual).
  // The <audio> element gets the squared value, remote outputs get the percent.
  const lastVolumeSentRef = useRef(null)
  const lastVolumeUserChangeRef = useRef(0)

  const { authenticated } = useAuthState()

  // Keep a ref to playerState so the mount effect can read the latest value
  // without re-triggering on every queue/position change
  const playerStateRef = useRef(playerState)
  playerStateRef.current = playerState

  currentTrackIdRef.current = currentTrackId
  outputDeviceRef.current = outputDevice
  remoteActiveRef.current = remoteActive

  const getReportExtra = useCallback(
    () => ({
      volume: Math.round(
        (playerStateRef.current?.volume ?? config.defaultUIVolume / 100) * 100,
      ),
      outputDevice: outputDeviceRef.current || BROWSER_DEVICE,
      playMode: playerStateRef.current?.mode || '',
      bilingualActive: !!playerStateRef.current?.bilingualActive,
    }),
    [],
  )

  useInterval(
    () => {
      if (heartbeatTrackId && !stoppedRef.current) {
        subsonic.reportPlayback(
          heartbeatTrackId,
          lastPositionMsRef.current,
          'playing',
          getReportExtra(),
        )
      }
    },
    heartbeatTrackId ? config.playbackReportIntervalMs : null,
  )

  // Detect browser codec profile and eagerly resolve transcode URLs for the
  // persisted queue once on mount (e.g. after a browser refresh)
  useEffect(() => {
    const profile = detectBrowserProfile()
    decisionService.setProfile(profile)
    dispatch(setTranscodingProfile(profile))

    const state = playerStateRef.current
    const currentIdx = state.savedPlayIndex || 0
    const trackIds = state.queue
      .slice(currentIdx, currentIdx + 4)
      .filter((item) => !item.isRadio && item.trackId)
      .map((item) => item.trackId)

    if (trackIds.length === 0) {
      dispatch(refreshQueue())
      return
    }

    Promise.allSettled(
      trackIds.map((id) =>
        decisionService.resolveStreamUrl(id).then((url) => [id, url]),
      ),
    ).then((results) => {
      const resolvedUrls = {}
      results.forEach((r) => {
        if (r.status === 'fulfilled') {
          resolvedUrls[r.value[0]] = r.value[1]
        }
      })
      dispatch(refreshQueue(resolvedUrls))
    })
  }, [dispatch])

  // Pre-fetch transcode decisions for next 2-3 songs when queue or position changes
  useEffect(() => {
    if (!playerState.queue.length) return

    const currentIdx = playerState.savedPlayIndex || 0
    const nextSongIds = playerState.queue
      .slice(currentIdx + 1, currentIdx + 4)
      .filter((item) => !item.isRadio)
      .map((item) => item.trackId)

    if (nextSongIds.length > 0) {
      decisionService.prefetchDecisions(nextSongIds)
    }
  }, [playerState.queue, playerState.savedPlayIndex])

  const visible = authenticated && playerState.queue.length > 0
  const isRadio = playerState.current?.isRadio || false
  const classes = useStyle({
    isRadio,
    visible,
    enableCoverAnimation: config.enableCoverAnimation,
  })
  const showNotifications = useSelector(
    (state) => state.settings.notifications || false,
  )
  const gainInfo = useSelector((state) => state.replayGain)
  const [context, setContext] = useState(null)
  const [gainNode, setGainNode] = useState(null)

  const notifyRemoteError = useCallback(
    (message, error) => {
      // eslint-disable-next-line no-console
      console.error(message, error)
      const now = Date.now()
      // Throttle notifications, the status poll may fail repeatedly
      if (now - lastRemoteErrorRef.current < 30000) {
        return
      }
      lastRemoteErrorRef.current = now
      if (showNotifications) {
        // The driver error text (HTTP status, UPnP fault) is the only clue the
        // device is misconfigured rather than merely offline
        const detail = error?.body || error?.message
        sendNotification(
          translate('jukebox.errorTitle'),
          detail ? `${message}: ${detail}` : message,
        )
      }
    },
    [showNotifications, translate],
  )

  useEffect(() => {
    if (
      context === null &&
      audioInstance &&
      config.enableReplayGain &&
      'AudioContext' in window &&
      (gainInfo.gainMode === 'album' || gainInfo.gainMode === 'track')
    ) {
      const ctx = new AudioContext()
      // we need this to support radios in firefox
      audioInstance.crossOrigin = 'anonymous'
      const source = ctx.createMediaElementSource(audioInstance)
      const gain = ctx.createGain()

      source.connect(gain)
      gain.connect(ctx.destination)

      setContext(ctx)
      setGainNode(gain)
    }
  }, [audioInstance, context, gainInfo.gainMode])

  useEffect(() => {
    if (gainNode) {
      const current = playerState.current || {}
      const song = current.song || {}

      const numericGain = calculateGain(gainInfo, song)
      gainNode.gain.setValueAtTime(numericGain, context.currentTime)
    }
  }, [audioInstance, context, gainNode, playerState, gainInfo])

  // When a remote output is active, keep the <audio> element playing but muted:
  // the player UI keeps a live clock/queue, while the sound comes from the
  // remote device
  useEffect(() => {
    if (!audioInstance) {
      return
    }
    audioInstance.muted = remoteActive
  }, [audioInstance, remoteActive])

  // Apply pending seek and state from takeover or remote synchronization
  useEffect(() => {
    if (playerState.pendingSeekTime != null && audioInstance) {
      const targetTime = playerState.pendingSeekTime
      const targetState = playerState.pendingState
      const applyHandoff = () => {
        if (targetTime > 0) {
          audioInstance.currentTime = targetTime
        }
        if (targetState === 'paused') {
          if (!audioInstance.paused) {
            audioInstance.pause()
          }
        } else if (targetState === 'playing') {
          if (audioInstance.paused) {
            audioInstance.play().catch(() => {})
          }
        }
        dispatch(clearPendingSeek())
      }

      if (audioInstance.readyState >= 1) {
        applyHandoff()
      } else {
        const onLoaded = () => {
          applyHandoff()
        }
        audioInstance.addEventListener('loadedmetadata', onLoaded, {
          once: true,
        })
        return () => {
          audioInstance.removeEventListener('loadedmetadata', onLoaded)
        }
      }
    }
  }, [
    playerState.pendingSeekTime,
    playerState.pendingState,
    audioInstance,
    dispatch,
  ])

  // Handle cross-client playback handoff (single-active-speaker mutual exclusion)
  const lastHandledHandoffRef = useRef(null)
  useEffect(() => {
    const handoff = playerState.lastHandoff
    if (!handoff || !handoff._receivedAt) return
    if (lastHandledHandoffRef.current === handoff._receivedAt) return
    lastHandledHandoffRef.current = handoff._receivedAt

    // Check if THIS client is the target that was taken over
    if (handoff.targetSessionId && handoff.targetSessionId === clientUniqueId) {
      isHandoffPausedRef.current = true
      if (audioInstance && !audioInstance.paused) {
        audioInstance.pause()
      }
      if (currentTrackIdRef.current) {
        const posMs = Math.floor((audioInstance?.currentTime || 0) * 1000)
        subsonic.reportPlayback(
          currentTrackIdRef.current,
          posMs,
          'paused',
          getReportExtra(),
        )
      }
      const remoteName =
        handoff.newPlayerName ||
        translate('nowPlaying.otherDevice') ||
        '其他设备'
      notify(
        translate('nowPlaying.handoffPausedNotice', { name: remoteName }) ||
          `播放已被「${remoteName}」接管，本地已暂停`,
        { type: 'info' },
      )
    }
  }, [
    playerState.lastHandoff,
    audioInstance,
    notify,
    translate,
    getReportExtra,
  ])

  // Take a remote output's reported volume (0-100) as the shared volume.
  // Zero is ignored: drivers report 0 until the device answers its first
  // volume query, and adopting that would silence the UI and get persisted.
  const adoptDeviceVolume = useCallback(
    (percent) => {
      if (typeof percent !== 'number' || percent <= 0 || percent > 100) {
        return
      }
      lastVolumeSentRef.current = percent
      const volume = percent / 100
      const stored = playerStateRef.current?.volume
      if (stored == null || Math.abs(stored - volume) > 0.005) {
        dispatch(setVolume(volume))
      }
    },
    [dispatch],
  )

  // Keyboard volume must go through the same store path as the slider, or the
  // authority effect below would revert it and a remote output would not follow.
  const adjustVolume = useCallback(
    (delta) => {
      const current = playerStateRef.current?.volume ?? 1
      dispatch(setVolume(Math.min(1, Math.max(0, current + delta))))
    },
    [dispatch],
  )

  // The store is the only volume authority for the <audio> element: the library
  // keeps its own level and re-applies it (fading from 0) whenever playback
  // starts, so the value is re-asserted on every change instead of read back.
  useEffect(() => {
    if (!audioInstance) {
      return
    }
    const applyToElement = () => {
      const volume = Math.min(1, Math.max(0, playerState.volume))
      const elementVolume = volume * volume
      if (Math.abs(audioInstance.volume - elementVolume) > 0.005) {
        audioInstance.volume = elementVolume
      }
    }
    applyToElement()
    audioInstance.addEventListener('volumechange', applyToElement)
    return () =>
      audioInstance.removeEventListener('volumechange', applyToElement)
  }, [audioInstance, playerState.volume])

  // Push the shared volume to the active remote output
  useEffect(() => {
    if (!remoteActive) {
      return
    }
    const volume = Math.min(1, Math.max(0, playerState.volume))
    const percent = Math.round(volume * 100)
    if (percent === lastVolumeSentRef.current) {
      return
    }
    lastVolumeUserChangeRef.current = Date.now()
    const timer = setTimeout(() => {
      lastVolumeSentRef.current = percent
      jukebox
        .control(outputDeviceRef.current, 'volume', percent)
        .catch((e) => notifyRemoteError(translate('jukebox.errorControl'), e))
    }, 200)
    return () => clearTimeout(timer)
  }, [playerState.volume, remoteActive, notifyRemoteError, translate])

  // Inform the server which output is active, and hand the current song over
  // when switching to a remote device mid-playback
  useEffect(() => {
    let cancelled = false
    const current = playerStateRef.current?.current
    const shouldTransfer =
      remoteActive && audioInstance && !audioInstance.paused && current?.trackId
    const position = Math.floor(audioInstance?.currentTime || 0)
    jukebox
      .selectDevice(outputDevice)
      .then(() => {
        if (cancelled) {
          return
        }
        remoteTrackRef.current = null
        remotePlayingRef.current = false
        remoteNoProgressRef.current = false
        remoteStatusFailuresRef.current = 0
        remoteStatusErrorShownRef.current = false
        setRemoteStatusDelay(1000)
        if (!remoteActive) {
          return
        }
        // Adopt the output's own level rather than pushing ours, so the same
        // speaker shows (and keeps) one volume across every client
        return jukebox
          .status()
          .then((state) => {
            if (!cancelled) {
              adoptDeviceVolume(state?.volume)
            }
          })
          .catch(() => {})
          .then(() => {
            if (cancelled || !shouldTransfer) {
              return
            }
            return jukebox
              .play(outputDevice, {
                songId: current.isRadio ? undefined : current.trackId,
                streamUrl: current.isRadio ? current.musicSrc : undefined,
                position:
                  !current.isRadio &&
                  position > 1 &&
                  isFinite(audioInstance.duration)
                    ? position
                    : undefined,
              })
              .then(() => {
                remoteTrackRef.current = current.trackId
                remotePlayingRef.current = true
              })
          })
      })
      .catch((e) => notifyRemoteError(translate('jukebox.errorSwitch'), e))
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [outputDevice, audioInstance, remoteActive])

  // Synchronize dynamic lyric updates (e.g. bilingual translation toggle) to player instance
  useEffect(() => {
    const currentLyric = playerState.current?.lyric
    const currentTrackId = playerState.current?.trackId
    const player = playerRef.current
    if (player && currentLyric !== undefined) {
      const playIndex =
        typeof player.getCurrentPlayIndex === 'function'
          ? player.getCurrentPlayIndex()
          : player.state?.playIndex || 0

      // 1. Update all matching tracks in player's internal state.audioLists so updateAudioLists won't revert
      if (Array.isArray(player.state?.audioLists)) {
        player.state.audioLists.forEach((item) => {
          if (item.trackId === currentTrackId) {
            item.lyric = currentLyric
          }
        })
        if (player.state.audioLists[playIndex]) {
          player.state.audioLists[playIndex].lyric = currentLyric
        }
      }

      // 2. Update player's internal state.lyric and force immediate re-parse
      if (player.state?.lyric !== currentLyric) {
        player.setState({ lyric: currentLyric }, () => {
          if (typeof player.initLyricParser === 'function') {
            player.initLyricParser()
          } else if (player.lyric && audioInstance) {
            player.lyric.update((audioInstance.currentTime || 0) * 1000)
          }
        })
      } else if (player.lyric && audioInstance) {
        player.lyric.update((audioInstance.currentTime || 0) * 1000)
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [playerState.current?.lyric, playerState.current?.trackId, audioInstance])

  // If bilingual mode was inherited during handoff, ensure the translated lyric is loaded
  const isBilingualActive = playerState.bilingualActive
  const currentTrackIdForLyric = playerState.current?.trackId
  const bilingualLyrics = playerState.bilingualLyrics
  useEffect(() => {
    if (
      isBilingualActive &&
      currentTrackIdForLyric &&
      !bilingualLyrics?.[currentTrackIdForLyric]
    ) {
      httpClient('/api/lyrics/translate', {
        method: 'POST',
        body: JSON.stringify({ songId: currentTrackIdForLyric }),
      })
        .then((res) => {
          const bilingualLrc =
            res.json?.inlineLrc ||
            res.json?.combinedLrc ||
            res.json?.bilingualLrc ||
            ''
          if (bilingualLrc) {
            dispatch(
              updateSongLyric(currentTrackIdForLyric, bilingualLrc, true),
            )
          }
        })
        .catch(() => {})
    }
  }, [isBilingualActive, currentTrackIdForLyric, bilingualLyrics, dispatch])

  useEffect(() => {
    const handleBeforeUnload = (e) => {
      if (playerState.current?.uuid && audioInstance && !audioInstance.paused) {
        e.preventDefault()
        e.returnValue = ''
      }
    }

    const handlePageHide = () => {
      if (currentTrackIdRef.current && !playerState.current?.isRadio) {
        stoppedRef.current = true
        try {
          subsonic.reportPlaybackKeepalive(
            currentTrackIdRef.current,
            lastPositionMsRef.current,
            'stopped',
            getReportExtra(),
          )
        } catch {
          // fetch/sendBeacon may throw; ignore
        }
      }
    }

    window.addEventListener('beforeunload', handleBeforeUnload)
    window.addEventListener('pagehide', handlePageHide)
    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload)
      window.removeEventListener('pagehide', handlePageHide)
    }
  }, [playerState, audioInstance, getReportExtra])

  const defaultOptions = useMemo(
    () => ({
      theme: playerTheme,
      bounds: 'body',
      playMode: playerState.mode,
      mode: 'full',
      loadAudioErrorPlayNext: false,
      autoPlayInitLoadPlayList: true,
      clearPriorAudioLists: false,
      showDestroy: true,
      showDownload: false,
      showLyric: true,
      showReload: false,
      toggleMode: false,
      responsive: false,
      glassBg: false,
      showThemeSwitch: false,
      showMediaSession: true,
      restartCurrentOnPrev: true,
      quietUpdate: true,
      defaultPosition: {
        top: 300,
        left: 120,
      },
      volumeFade: { fadeIn: 200, fadeOut: 200 },
      renderAudioTitle: (audioInfo, isMobile) => (
        <AudioTitle
          audioInfo={audioInfo}
          gainInfo={gainInfo}
          isMobile={isMobile}
        />
      ),
      locale: locale(translate),
      sortableOptions: { delay: 200, delayOnTouchOnly: true },
    }),
    [gainInfo, playerTheme, translate, playerState.mode],
  )

  const options = useMemo(() => {
    const current = playerState.current || {}
    return {
      ...defaultOptions,
      audioLists: playerState.queue.map((item) => item),
      playIndex: playerState.playIndex,
      autoPlay:
        playerState.queue.length > 0 &&
        playerState.autoPlay !== false &&
        playerState.pendingState !== 'paused' &&
        (playerState.clear || playerState.playIndex === 0),
      clearPriorAudioLists: playerState.clear,
      extendsContent: (
        <PlayerToolbar id={current.trackId} isRadio={current.isRadio} />
      ),
      defaultVolume: playerState.volume,
      showMediaSession: !current.isRadio,
    }
  }, [playerState, defaultOptions])

  const onAudioListsChange = useCallback(
    (_, audioLists, audioInfo) => dispatch(syncQueue(audioInfo, audioLists)),
    [dispatch],
  )

  const onAudioProgress = useCallback((info) => {
    if (info.ended) {
      document.title = 'Navidrome'
    }
    if (!info.isRadio && info.currentTime != null) {
      lastPositionMsRef.current = Math.floor(info.currentTime * 1000)
    }
  }, [])

  const onAudioPlay = useCallback(
    (info) => {
      isHandoffPausedRef.current = false
      if (context && context.state !== 'running') {
        context.resume()
      }

      if (playerStateRef.current?.pendingState === 'paused' && audioInstance) {
        audioInstance.pause()
        return
      }

      dispatch(currentPlaying(info))
      if (info.duration) {
        const song = info.song
        document.title = `${song.title} - ${song.artist} - Navidrome`
        if (!info.isRadio) {
          const posMs = Math.floor(info.currentTime * 1000)
          lastPositionMsRef.current = posMs
          const isNewTrack = info.trackId !== currentTrackId
          if (isNewTrack) {
            subsonic
              .reportPlayback(info.trackId, posMs, 'starting', getReportExtra())
              .then(() =>
                subsonic.reportPlayback(
                  info.trackId,
                  posMs,
                  'playing',
                  getReportExtra(),
                ),
              )
            setCurrentTrackId(info.trackId)
          } else {
            subsonic.reportPlayback(
              info.trackId,
              posMs,
              'playing',
              getReportExtra(),
            )
          }
          setHeartbeatTrackId(info.trackId)
        }
        if (config.gaTrackingId) {
          ReactGA.event({
            category: 'Player',
            action: 'Play song',
            label: `${song.title} - ${song.artist}`,
          })
        }
        if (showNotifications) {
          sendNotification(
            song.title,
            `${song.artist} - ${song.album}`,
            info.cover,
          )
        }
      }

      // Forward playback to the selected remote output
      lastUserPlayRef.current = Date.now()
      if (remoteActiveRef.current) {
        const device = outputDeviceRef.current
        if (info.trackId && info.trackId !== remoteTrackRef.current) {
          remoteTrackRef.current = info.trackId
          remotePlayingRef.current = false
          // The local clock may already be mid-track (e.g. the output was
          // switched while paused), so hand the position over with the song
          const position =
            !info.isRadio && isFinite(info.duration) && info.currentTime > 1
              ? Math.floor(info.currentTime)
              : undefined
          jukebox
            .play(device, {
              songId: info.isRadio ? undefined : info.trackId,
              streamUrl: info.isRadio ? info.musicSrc : undefined,
              position,
            })
            .catch((e) =>
              notifyRemoteError(translate('jukebox.errorSwitch'), e),
            )
        } else if (remoteTrackRef.current) {
          jukebox
            .control(device, 'resume')
            .catch((e) =>
              notifyRemoteError(translate('jukebox.errorControl'), e),
            )
        }
      }
    },
    [
      context,
      dispatch,
      showNotifications,
      currentTrackId,
      notifyRemoteError,
      translate,
      audioInstance,
      getReportExtra,
    ],
  )

  const onAudioPlayTrackChange = useCallback(() => {
    if (currentTrackId) {
      subsonic.reportPlayback(
        currentTrackId,
        lastPositionMsRef.current,
        'stopped',
      )
    }
    setHeartbeatTrackId(null)
    setCurrentTrackId(null)
  }, [currentTrackId])

  const onAudioPause = useCallback(
    (info) => {
      dispatch(currentPlaying(info))
      if (!info.isRadio && currentTrackId) {
        const posMs = Math.floor(info.currentTime * 1000)
        lastPositionMsRef.current = posMs
        subsonic.reportPlayback(
          currentTrackId,
          posMs,
          'paused',
          getReportExtra(),
        )
      }
      setHeartbeatTrackId(null)
      lastUserPauseRef.current = Date.now()
      if (remoteActiveRef.current && remoteTrackRef.current) {
        if (!isHandoffPausedRef.current) {
          jukebox
            .control(outputDeviceRef.current, 'pause')
            .catch((e) =>
              notifyRemoteError(translate('jukebox.errorControl'), e),
            )
        }
      }
    },
    [dispatch, currentTrackId, notifyRemoteError, translate, getReportExtra],
  )

  const onAudioEnded = useCallback(
    (currentPlayId, audioLists, info) => {
      if (currentTrackId && !info.isRadio) {
        const posMs = Math.floor((info.duration || 0) * 1000)
        subsonic.reportPlayback(
          currentTrackId,
          posMs,
          'stopped',
          getReportExtra(),
        )
      }
      setHeartbeatTrackId(null)
      setCurrentTrackId(null)
      remoteTrackRef.current = null
      remotePlayingRef.current = false
      dispatch(currentPlaying(info))
      dataProvider
        .getOne('keepalive', { id: info.trackId })
        // eslint-disable-next-line no-console
        .catch((e) => console.log('Keepalive error:', e))
    },
    [dispatch, dataProvider, currentTrackId, getReportExtra],
  )

  const onCoverClick = useCallback((mode, audioLists, audioInfo) => {
    if (mode === 'full' && audioInfo?.song?.albumId) {
      window.location.href = `#/album/${audioInfo.song.albumId}/show`
    }
  }, [])

  const onAudioError = useCallback(
    (error, currentPlayId, audioLists, audioInfo) => {
      // Invalidate all cached decisions — token may be stale
      decisionService.invalidateAll()

      // Pre-fetch decisions for upcoming songs with fresh tokens
      const currentIdx = playerState.queue.findIndex(
        (item) => item.uuid === currentPlayId,
      )
      if (currentIdx >= 0) {
        const nextSongIds = playerState.queue
          .slice(currentIdx + 1, currentIdx + 4)
          .filter((item) => !item.isRadio)
          .map((item) => item.trackId)
        if (nextSongIds.length > 0) {
          decisionService.prefetchDecisions(nextSongIds)
        }
      }
    },
    [playerState.queue],
  )

  const onBeforeDestroy = useCallback(() => {
    return new Promise((resolve, reject) => {
      if (currentTrackId && !playerStateRef.current?.current?.isRadio) {
        subsonic.reportPlayback(
          currentTrackId,
          lastPositionMsRef.current,
          'stopped',
          getReportExtra(),
        )
      }
      setHeartbeatTrackId(null)
      setCurrentTrackId(null)
      if (remoteActiveRef.current) {
        // The remote output keeps playing on its own while the local player is
        // destroyed, so ask it to stop
        jukebox.control(outputDeviceRef.current, 'stop').catch(() => {})
        remoteTrackRef.current = null
        remotePlayingRef.current = false
      }
      dispatch(clearQueue())
      reject()
    })
  }, [dispatch, currentTrackId, getReportExtra])

  if (!visible) {
    document.title = 'Navidrome'
  }

  const handlers = useMemo(
    () => keyHandlers(audioInstance, playerState, adjustVolume),
    [audioInstance, playerState, adjustVolume],
  )

  // Report every seek (including programmatic ones the library does not surface
  // via onAudioSeeked, e.g. restartCurrentOnPrev). Debounce coalesces drag
  // bursts into one report at the final position.
  useEffect(() => {
    if (!audioInstance) return
    let timer = null
    const flush = () => {
      timer = null
      const isDriftCorrection = suppressSeekRef.current
      suppressSeekRef.current = false
      if (
        remoteActiveRef.current &&
        remoteTrackRef.current &&
        !isDriftCorrection &&
        !remoteNoProgressRef.current
      ) {
        // User-initiated seek: forward it to the remote output
        jukebox
          .control(
            outputDeviceRef.current,
            'seek',
            Math.max(0, Math.floor(audioInstance.currentTime || 0)),
          )
          .catch(() => {})
      }
      if (
        !currentTrackIdRef.current ||
        playerStateRef.current?.current?.isRadio
      ) {
        return
      }
      const posMs = Math.floor((audioInstance.currentTime || 0) * 1000)
      const state = audioInstance.paused ? 'paused' : 'playing'
      subsonic.reportPlayback(
        currentTrackIdRef.current,
        posMs,
        state,
        getReportExtra(),
      )
    }
    const handleSeeked = () => {
      if (timer) clearTimeout(timer)
      timer = setTimeout(flush, 250)
    }
    audioInstance.addEventListener('seeked', handleSeeked)
    return () => {
      if (timer) clearTimeout(timer)
      audioInstance.removeEventListener('seeked', handleSeeked)
    }
  }, [audioInstance, getReportExtra])

  // Keep the local clock in sync with the remote output: correct position
  // drift, and follow play/pause/stop performed directly on the device
  useInterval(
    () => {
      if (!remoteActiveRef.current || !audioInstance) {
        return
      }
      jukebox
        .status()
        .then((state) => {
          if (!remoteActiveRef.current || !audioInstance) {
            return
          }
          if (remoteStatusFailuresRef.current !== 0) {
            remoteStatusFailuresRef.current = 0
            remoteStatusErrorShownRef.current = false
            setRemoteStatusDelay(1000)
          }
          // Devices without progress reporting (e.g. xiaomi speakers) must
          // not trigger drift corrections or seek forwarding
          remoteNoProgressRef.current = state.deviceType === 'xiaomi'
          // Follow the output's own level (physical buttons, voice command),
          // but never fight a slider drag that is still in flight
          if (
            typeof state.volume === 'number' &&
            Date.now() - lastVolumeUserChangeRef.current > 3000
          ) {
            adoptDeviceVolume(state.volume)
          }
          const isRadio = playerStateRef.current?.current?.isRadio
          const remoteTime = state.currentTime || 0
          switch (state.status) {
            case 'playing': {
              remotePlayingRef.current = true
              if (audioInstance.paused) {
                // If this client was taken over by another device, do not auto-resume
                if (isHandoffPausedRef.current) {
                  break
                }
                // Resumed directly on the device (voice command, device button)
                if (Date.now() - lastUserPauseRef.current > 3000) {
                  audioInstance.play().catch(() => {})
                }
              } else if (!isRadio && !remoteNoProgressRef.current) {
                const drift = remoteTime - (audioInstance.currentTime || 0)
                if (Math.abs(drift) > 2.5 && drift > -10) {
                  // Position drift: make the remote position authoritative, but
                  // mark it so the change is not echoed back as a user seek.
                  // Large backwards jumps are ignored: renderers such as the
                  // Xiaomi S12 restart their RelTime from 0 while audio plays.
                  suppressSeekRef.current = true
                  audioInstance.currentTime = Math.max(0, remoteTime)
                  window.setTimeout(() => {
                    suppressSeekRef.current = false
                  }, 1000)
                }
              }
              break
            }
            case 'paused':
              if (
                !audioInstance.paused &&
                Date.now() - lastUserPlayRef.current > 3000
              ) {
                // Paused directly on the device
                audioInstance.pause()
              }
              break
            default:
              // Remote stopped while we thought it was playing: the track
              // finished (or was stopped on the device). Advance the local
              // player to the end, so the queue keeps moving.
              // For devices without progress reporting (e.g. xiaomi), the local
              // muted audioInstance maintains the clock and handles song completion
              // via onAudioEnded. We must not force-skip on transient status.
              if (
                remotePlayingRef.current &&
                !audioInstance.paused &&
                !remoteNoProgressRef.current
              ) {
                remotePlayingRef.current = false
                remoteTrackRef.current = null
                const duration = audioInstance.duration
                if (!isRadio && isFinite(duration) && duration > 0) {
                  suppressSeekRef.current = true
                  audioInstance.currentTime = Math.max(0, duration - 0.2)
                  window.setTimeout(() => {
                    suppressSeekRef.current = false
                  }, 1000)
                } else {
                  audioInstance.pause()
                }
              }
              break
          }
        })
        .catch((e) => {
          remoteStatusFailuresRef.current += 1
          const failures = remoteStatusFailuresRef.current
          const delay = failures >= 8 ? 15000 : failures >= 4 ? 5000 : 1000
          setRemoteStatusDelay((prev) => (prev === delay ? prev : delay))
          // Report the episode once; the backoff already tells the user the
          // device is not answering
          if (!remoteStatusErrorShownRef.current) {
            remoteStatusErrorShownRef.current = true
            notifyRemoteError(translate('jukebox.errorStatus'), e)
          }
        })
    },
    remoteActive && audioInstance ? remoteStatusDelay : null,
  )

  return (
    <ThemeProvider theme={createMuiTheme(theme)}>
      <ReactJkMusicPlayer
        ref={playerRef}
        {...options}
        className={classes.player}
        onAudioListsChange={onAudioListsChange}
        onAudioProgress={onAudioProgress}
        onAudioPlay={onAudioPlay}
        onAudioPlayTrackChange={onAudioPlayTrackChange}
        onAudioPause={onAudioPause}
        onPlayModeChange={(mode) => dispatch(setPlayMode(mode))}
        onAudioEnded={onAudioEnded}
        onCoverClick={onCoverClick}
        onAudioError={onAudioError}
        onBeforeDestroy={onBeforeDestroy}
        getAudioInstance={setAudioInstance}
      />
      <GlobalHotKeys handlers={handlers} keyMap={keyMap} allowChanges />
    </ThemeProvider>
  )
}

export { Player }
