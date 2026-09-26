import React, { useState, useEffect, useCallback, useRef } from 'react'
import PropTypes from 'prop-types'
import { useSelector, useDispatch } from 'react-redux'
import { useTranslate, Link, useNotify } from 'react-admin'
import {
  Popover,
  IconButton,
  makeStyles,
  Tooltip,
  List,
  ListItem,
  Avatar,
  Badge,
  Card,
  CardContent,
  Typography,
  LinearProgress,
  useTheme,
  useMediaQuery,
  Chip,
} from '@material-ui/core'
import { FaRegCirclePlay, FaPause, FaPlay } from 'react-icons/fa6'
import { RiSpeaker2Line } from 'react-icons/ri'
import subsonic from '../subsonic'
import httpClient, { clientUniqueId } from '../dataProvider/httpClient'
import { useInterval } from '../common'
import { nowPlayingCountSync, takeoverTrack } from '../actions'
import { formatDuration, formatDeviceName } from '../utils'
import config from '../config'
import * as jukebox from '../audioplayer/jukebox'

const useStyles = makeStyles((theme) => ({
  button: { color: 'inherit' },
  list: {
    width: '26em',
    maxHeight: (props) => {
      const entryHeight = 120
      const maxEntries = Math.min(props.entryCount || 0, 3)
      return maxEntries > 0 ? `${maxEntries * entryHeight}px` : '12em'
    },
    overflowY: 'auto',
    padding: 0,
  },
  card: {
    padding: 0,
  },
  cardContent: {
    padding: `${theme.spacing(1)}px !important`,
    '&:last-child': {
      paddingBottom: `${theme.spacing(1)}px !important`,
    },
  },
  listItem: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1.5),
    padding: theme.spacing(1),
    cursor: 'pointer',
    position: 'relative',
    transition: 'background-color 0.15s ease',
    borderRadius: theme.spacing(0.5),
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
    '&:hover $takeoverButton': {
      opacity: 1,
    },
  },
  avatarContainer: {
    position: 'relative',
    flexShrink: 0,
    width: theme.spacing(8),
    height: theme.spacing(8),
  },
  avatar: {
    width: '100%',
    height: '100%',
    cursor: 'pointer',
    borderRadius: theme.spacing(0.5),
    '&:hover': {
      opacity: 0.8,
    },
  },
  stateOverlay: {
    position: 'absolute',
    top: 0,
    left: 0,
    width: '100%',
    height: '100%',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: 'rgba(0, 0, 0, 0.45)',
    borderRadius: theme.spacing(0.5),
    pointerEvents: 'none',
  },
  stateIcon: {
    color: 'rgba(255, 255, 255, 0.85)',
    fontSize: 18,
  },
  entryContent: {
    flex: 1,
    minWidth: 0,
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(0.25),
  },
  trackTitle: {
    fontWeight: 600,
    fontSize: '0.875rem',
    lineHeight: 1.3,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  trackDetail: {
    fontSize: '0.75rem',
    color: theme.palette.text.secondary,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  artistLink: {
    cursor: 'pointer',
    color: theme.palette.text.secondary,
    fontSize: '0.75rem',
    '&:hover': {
      textDecoration: 'underline',
    },
  },
  progressRow: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.75),
    marginTop: theme.spacing(0.5),
  },
  progressTime: {
    fontSize: '0.65rem',
    color: theme.palette.text.secondary,
    fontVariantNumeric: 'tabular-nums',
    flexShrink: 0,
  },
  progressBar: {
    flex: 1,
    height: 3,
    borderRadius: 2,
    backgroundColor: theme.palette.action.disabledBackground,
    '& .MuiLinearProgress-bar': {
      borderRadius: 2,
    },
  },
  userInfo: {
    fontSize: '0.65rem',
    color: theme.palette.text.disabled,
    marginTop: theme.spacing(0.25),
  },
  takeoverButton: {
    opacity: 0.7,
    padding: theme.spacing(0.75),
    color: theme.palette.primary.main,
    transition: 'opacity 0.2s, transform 0.15s',
    alignSelf: 'center',
    flexShrink: 0,
    '&:hover': {
      opacity: 1,
      transform: 'scale(1.15)',
    },
  },
  badge: {
    '& .MuiBadge-badge': {
      backgroundColor: theme.palette.primary.main,
      color: theme.palette.primary.contrastText,
    },
  },
  currentDeviceBadge: {
    marginLeft: theme.spacing(0.75),
    padding: '1px 5px',
    borderRadius: 3,
    fontSize: '0.625rem',
    fontWeight: 600,
    backgroundColor: theme.palette.primary.main,
    color: theme.palette.primary.contrastText,
    display: 'inline-block',
    lineHeight: '1.3',
    verticalAlign: 'middle',
  },
  remoteOutputBadge: {
    marginLeft: theme.spacing(0.75),
    padding: '1px 6px',
    borderRadius: 3,
    fontSize: '0.625rem',
    fontWeight: 600,
    backgroundColor: theme.palette.secondary.main,
    color: theme.palette.secondary.contrastText,
    display: 'inline-flex',
    alignItems: 'center',
    gap: 3,
    lineHeight: '1.3',
    verticalAlign: 'middle',
  },
  speakerIcon: {
    fontSize: '0.75rem',
    flexShrink: 0,
  },
  volumeBadge: {
    marginLeft: theme.spacing(0.5),
    padding: '1px 4px',
    borderRadius: 3,
    fontSize: '0.6rem',
    fontWeight: 500,
    backgroundColor: theme.palette.action.selected,
    color: theme.palette.text.secondary,
    display: 'inline-block',
    lineHeight: '1.3',
    verticalAlign: 'middle',
  },
  currentDeviceChipWrapper: {
    alignSelf: 'center',
    flexShrink: 0,
    padding: theme.spacing(0.25),
  },
  currentDeviceChip: {
    height: 22,
    fontSize: '0.7rem',
    fontWeight: 500,
    borderColor: theme.palette.primary.main,
    color: theme.palette.primary.main,
  },
}))

// NowPlayingButton component - handles the button with badge
const NowPlayingButton = React.memo(({ count, onClick }) => {
  const classes = useStyles()
  const translate = useTranslate()

  return (
    <Tooltip title={translate('nowPlaying.title')}>
      <IconButton
        className={classes.button}
        onClick={onClick}
        aria-label={translate('nowPlaying.title')}
        aria-haspopup="true"
      >
        <Badge
          badgeContent={count}
          color="primary"
          overlap="rectangular"
          className={classes.badge}
        >
          <FaRegCirclePlay size={20} />
        </Badge>
      </IconButton>
    </Tooltip>
  )
})

NowPlayingButton.displayName = 'NowPlayingButton'

NowPlayingButton.propTypes = {
  count: PropTypes.number.isRequired,
  onClick: PropTypes.func.isRequired,
}

const NowPlayingItem = React.memo(
  ({
    nowPlayingEntry,
    deviceNames,
    currentVolume,
    outputDevice,
    onLinkClick,
    getArtistLink,
    onTakeover,
    now,
  }) => {
    const classes = useStyles()
    const translate = useTranslate()
    const isPaused = nowPlayingEntry.state === 'paused'
    const isPlaying =
      nowPlayingEntry.state === 'playing' ||
      nowPlayingEntry.state === 'starting'
    const basePositionMs = nowPlayingEntry.positionMs || 0
    const rate = nowPlayingEntry.playbackRate || 1
    const elapsedSinceFetch = now - (nowPlayingEntry._fetchedAt || now)
    const interpolatedMs = isPlaying
      ? basePositionMs + elapsedSinceFetch * rate
      : basePositionMs
    const durationMs = (nowPlayingEntry.duration || 0) * 1000
    const clampedMs = Math.max(0, interpolatedMs)
    const positionMs =
      durationMs > 0 ? Math.min(clampedMs, durationMs) : clampedMs
    const positionSec = Math.floor(positionMs / 1000)
    const durationSec = nowPlayingEntry.duration || 0
    const progress =
      durationSec > 0 ? (positionMs / 1000 / durationSec) * 100 : 0
    const artistId = nowPlayingEntry.albumArtistId || nowPlayingEntry.artistId
    const artistName = nowPlayingEntry.albumArtist || nowPlayingEntry.artist

    const isCurrent =
      Boolean(nowPlayingEntry.isCurrentSession) ||
      (nowPlayingEntry.sessionId &&
        nowPlayingEntry.sessionId === clientUniqueId) ||
      (nowPlayingEntry.playerId && nowPlayingEntry.playerId === clientUniqueId)

    const displayDevice = formatDeviceName(nowPlayingEntry.playerName)
    const effectiveOutputDevice = isCurrent
      ? outputDevice || nowPlayingEntry.outputDevice
      : nowPlayingEntry.outputDevice
    const isRemoteOutput = Boolean(
      effectiveOutputDevice && effectiveOutputDevice !== 'browser',
    )
    const outputDeviceName = isRemoteOutput
      ? (deviceNames && deviceNames[effectiveOutputDevice]) ||
        effectiveOutputDevice
      : ''
    const displayVolume =
      isCurrent && typeof currentVolume === 'number'
        ? Math.round(currentVolume * 100)
        : nowPlayingEntry.volume

    const handleItemClick = () => {
      if (isCurrent) return
      onTakeover(nowPlayingEntry, positionSec, nowPlayingEntry.state)
    }

    const handleButtonClick = (e) => {
      e.stopPropagation()
      if (isCurrent) return
      onTakeover(nowPlayingEntry, positionSec, 'playing')
    }

    const handleAvatarClick = (e) => {
      e.stopPropagation()
      onLinkClick()
    }

    const handleArtistClick = (e) => {
      e.stopPropagation()
      onLinkClick()
    }

    return (
      <ListItem className={classes.listItem} onClick={handleItemClick}>
        <div className={classes.avatarContainer}>
          <Link
            to={`/album/${nowPlayingEntry.albumId}/show`}
            onClick={handleAvatarClick}
          >
            <Avatar
              className={classes.avatar}
              src={subsonic.getCoverArtUrl(nowPlayingEntry, 80)}
              variant="square"
              alt={`${nowPlayingEntry.album} cover art`}
              loading="lazy"
            />
          </Link>
          {isPaused && (
            <div className={classes.stateOverlay}>
              <FaPause className={classes.stateIcon} />
            </div>
          )}
        </div>
        <div className={classes.entryContent}>
          <Typography
            className={classes.trackTitle}
            title={nowPlayingEntry.title}
          >
            {nowPlayingEntry.title}
          </Typography>
          {artistId ? (
            <Link
              to={getArtistLink(artistId)}
              className={classes.artistLink}
              onClick={handleArtistClick}
            >
              {artistName}
            </Link>
          ) : (
            <Typography className={classes.trackDetail}>
              {artistName}
            </Typography>
          )}
          <Typography
            className={classes.trackDetail}
            title={nowPlayingEntry.album}
          >
            {nowPlayingEntry.album}
          </Typography>
          <div className={classes.progressRow}>
            <span className={classes.progressTime}>
              {formatDuration(positionSec)}
            </span>
            <LinearProgress
              className={classes.progressBar}
              variant="determinate"
              value={Math.min(progress, 100)}
            />
            <span className={classes.progressTime}>
              {formatDuration(durationSec)}
            </span>
          </div>
          <Typography className={classes.userInfo}>
            {nowPlayingEntry.username}
            {displayDevice ? ` (${displayDevice})` : ''}
            {isCurrent && (
              <span className={classes.currentDeviceBadge}>
                {translate('nowPlaying.currentDeviceShort') || '本机'}
              </span>
            )}
            {isRemoteOutput && (
              <span
                className={classes.remoteOutputBadge}
                title={translate('nowPlaying.remoteOutputDevice') || '输出设备'}
              >
                <RiSpeaker2Line className={classes.speakerIcon} />
                <span>{outputDeviceName}</span>
              </span>
            )}
            {typeof displayVolume === 'number' && displayVolume > 0 && (
              <span
                className={classes.volumeBadge}
                title={translate('nowPlaying.volume') || '音量'}
              >
                {`${displayVolume}%`}
              </span>
            )}
          </Typography>
        </div>
        {isCurrent ? (
          <Tooltip
            title={
              translate('nowPlaying.currentlyPlayingHere') || '当前正在本机播放'
            }
          >
            <span className={classes.currentDeviceChipWrapper}>
              <Chip
                size="small"
                label={translate('nowPlaying.currentDeviceShort') || '本机'}
                color="primary"
                variant="outlined"
                className={classes.currentDeviceChip}
              />
            </span>
          </Tooltip>
        ) : (
          <Tooltip
            title={
              translate('nowPlaying.takeoverTooltip') || '在此设备接管播放'
            }
          >
            <IconButton
              size="small"
              className={classes.takeoverButton}
              onClick={handleButtonClick}
              aria-label={
                translate('nowPlaying.takeoverTooltip') || '在此设备接管播放'
              }
            >
              <FaPlay size={13} />
            </IconButton>
          </Tooltip>
        )}
      </ListItem>
    )
  },
)

NowPlayingItem.displayName = 'NowPlayingItem'

NowPlayingItem.propTypes = {
  nowPlayingEntry: PropTypes.shape({
    id: PropTypes.oneOfType([PropTypes.string, PropTypes.number]),
    songId: PropTypes.string,
    playerId: PropTypes.oneOfType([PropTypes.string, PropTypes.number]),
    sessionId: PropTypes.string,
    isCurrentSession: PropTypes.bool,
    albumId: PropTypes.oneOfType([PropTypes.string, PropTypes.number]),
    albumArtistId: PropTypes.oneOfType([PropTypes.string, PropTypes.number]),
    artistId: PropTypes.oneOfType([PropTypes.string, PropTypes.number]),
    albumArtist: PropTypes.string,
    artist: PropTypes.string,
    title: PropTypes.string.isRequired,
    username: PropTypes.string.isRequired,
    playerName: PropTypes.string,
    album: PropTypes.string,
    state: PropTypes.string,
    positionMs: PropTypes.number,
    duration: PropTypes.number,
    updatedAt: PropTypes.oneOfType([PropTypes.string, PropTypes.number]),
    outputDevice: PropTypes.string,
    volume: PropTypes.number,
    playMode: PropTypes.string,
    bilingual: PropTypes.bool,
  }).isRequired,
  deviceNames: PropTypes.object,
  currentVolume: PropTypes.number,
  outputDevice: PropTypes.string,
  onLinkClick: PropTypes.func.isRequired,
  getArtistLink: PropTypes.func.isRequired,
  onTakeover: PropTypes.func.isRequired,
  now: PropTypes.number.isRequired,
}

// NowPlayingList component - handles the popover content
const NowPlayingList = React.memo(
  ({
    anchorEl,
    open,
    onClose,
    entries,
    deviceNames,
    currentVolume,
    outputDevice,
    onLinkClick,
    getArtistLink,
    onTakeover,
    now,
  }) => {
    const classes = useStyles({ entryCount: entries.length })
    const translate = useTranslate()

    return (
      <Popover
        id="panel-nowplaying"
        anchorEl={anchorEl}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
        open={open}
        onClose={onClose}
        aria-labelledby="now-playing-title"
      >
        <Card className={classes.card}>
          <CardContent className={classes.cardContent}>
            {entries.length === 0 ? (
              <Typography id="now-playing-title">
                {translate('nowPlaying.empty')}
              </Typography>
            ) : (
              <List
                className={classes.list}
                dense
                aria-label={translate('nowPlaying.title')}
              >
                {entries.map((nowPlayingEntry) => (
                  <NowPlayingItem
                    key={`${nowPlayingEntry.sessionId || nowPlayingEntry.playerId || ''}-${nowPlayingEntry.username}-${nowPlayingEntry.playerName}-${nowPlayingEntry.id || nowPlayingEntry.songId || nowPlayingEntry.title}`}
                    nowPlayingEntry={nowPlayingEntry}
                    deviceNames={deviceNames}
                    currentVolume={currentVolume}
                    outputDevice={outputDevice}
                    onLinkClick={onLinkClick}
                    getArtistLink={getArtistLink}
                    onTakeover={onTakeover}
                    now={now}
                  />
                ))}
              </List>
            )}
          </CardContent>
        </Card>
      </Popover>
    )
  },
)

NowPlayingList.displayName = 'NowPlayingList'

NowPlayingList.propTypes = {
  anchorEl: PropTypes.object,
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  entries: PropTypes.arrayOf(PropTypes.object).isRequired,
  deviceNames: PropTypes.object,
  currentVolume: PropTypes.number,
  outputDevice: PropTypes.string,
  onLinkClick: PropTypes.func.isRequired,
  getArtistLink: PropTypes.func.isRequired,
  onTakeover: PropTypes.func.isRequired,
  now: PropTypes.number.isRequired,
}

// Main NowPlayingPanel component
const NowPlayingPanel = () => {
  const dispatch = useDispatch()
  const count = useSelector((state) => state.activity.nowPlayingCount)
  const lastUpdate = useSelector((state) => state.activity.nowPlayingLastUpdate)
  const streamReconnected = useSelector(
    (state) => state.activity.streamReconnected,
  )
  const serverUp = useSelector(
    (state) => !!state.activity.serverStart.startTime,
  )
  const currentVolume = useSelector((state) => state.player?.volume)
  const outputDevice =
    useSelector((state) => state.player?.outputDevice) || 'browser'
  const translate = useTranslate()
  const notify = useNotify()
  const theme = useTheme()
  const isSmallScreen = useMediaQuery(theme.breakpoints.down('sm'))

  const [anchorEl, setAnchorEl] = useState(null)
  const [entries, setEntries] = useState([])
  const [deviceNames, setDeviceNames] = useState({})
  const [now, setNow] = useState(Date.now())
  const open = Boolean(anchorEl)

  const loadDeviceNames = useCallback(() => {
    if (config.jukeboxEnabled) {
      jukebox
        .fetchDevices()
        .then((data) => {
          const map = {}
          ;(data?.devices || []).forEach((d) => {
            map[d.id] = d.name
          })
          setDeviceNames(map)
        })
        .catch(() => {})
    }
  }, [])

  useEffect(() => {
    loadDeviceNames()
  }, [loadDeviceNames])

  const handleToggle = useCallback(
    (event) => {
      const target = event.currentTarget
      setAnchorEl((prev) => {
        if (prev) return null
        loadDeviceNames()
        return target
      })
    },
    [loadDeviceNames],
  )

  const handleMenuClose = useCallback(() => {
    setAnchorEl(null)
  }, [])

  // Close panel when link is clicked on small screens
  const handleLinkClick = useCallback(() => {
    if (isSmallScreen) {
      handleMenuClose()
    }
  }, [isSmallScreen, handleMenuClose])

  const getArtistLink = useCallback((artistId) => {
    if (!artistId) return null
    return config.devShowArtistPage && artistId !== config.variousArtistsId
      ? `/artist/${artistId}/show`
      : `/album?filter={"artist_id":"${artistId}"}&order=ASC&sort=max_year&displayedFilters={"compilation":true}&perPage=15`
  }, [])

  const handleTakeover = useCallback(
    async (entry, posSec, requestedState) => {
      try {
        let songData = null
        try {
          const songId = entry.id || entry.songId
          if (songId) {
            const res = await subsonic.getSong(songId)
            const data = res?.json?.['subsonic-response']
            if (data?.status === 'ok' && data.song) {
              songData = data.song
            }
          }
        } catch {
          // ignore
        }

        if (!songData) {
          songData = {
            id: entry.id || entry.songId,
            title: entry.title,
            artist: entry.artist,
            album: entry.album,
            albumId: entry.albumId,
            duration: entry.duration,
            updatedAt: entry.updatedAt,
          }
        }

        const inheritedOutput = entry.outputDevice || 'browser'
        const inheritedVolume =
          typeof entry.volume === 'number' && entry.volume > 0
            ? entry.volume / 100
            : undefined
        const targetState = requestedState || entry.state || 'playing'
        const playMode = entry.playMode
        const bilingualActive =
          entry.bilingual != null ? entry.bilingual : entry.bilingualActive

        dispatch(
          takeoverTrack(songData, posSec, {
            outputDevice: inheritedOutput,
            volume: inheritedVolume,
            state: targetState,
            playMode,
            bilingualActive,
          }),
        )

        // If target output device is remote, ensure it is selected on server
        if (inheritedOutput && inheritedOutput !== 'browser') {
          jukebox.selectDevice(inheritedOutput).catch(() => {})
        }

        const targetSessionId = entry.sessionId || entry.playerId
        if (targetSessionId) {
          httpClient(
            `/api/playback/sessions/${encodeURIComponent(targetSessionId)}/takeover`,
            {
              method: 'POST',
              body: JSON.stringify({
                action: 'pause',
                sourceSessionId: clientUniqueId,
                newPlayerName: translate('nowPlaying.webPlayer') || '网页端',
                targetOutput: inheritedOutput,
              }),
            },
          )
            .then(() => {
              if (doFetchRef.current) {
                setTimeout(doFetchRef.current, 250)
              }
            })
            .catch(() => {})
        }

        // Optimistically update entries and badge count so the change is instant without page reload
        setEntries((prev) => {
          const next = prev.filter(
            (e) => (e.sessionId || e.playerId) !== targetSessionId,
          )
          dispatch(nowPlayingCountSync({ count: next.length }))
          return next
        })

        notify(
          translate('nowPlaying.takeoverSuccess', {
            title: entry.title,
            time: formatDuration(posSec),
          }) || `已从 ${formatDuration(posSec)} 接管播放：${entry.title}`,
          { type: 'info' },
        )
        handleMenuClose()
      } catch (err) {
        notify('ra.page.error', {
          type: 'warning',
          messageArgs: { error: err.message || 'Takeover failed' },
        })
      }
    },
    [dispatch, notify, translate, handleMenuClose],
  )

  const fetchTimerRef = useRef(null)
  const doFetchRef = useRef()
  doFetchRef.current = () =>
    subsonic
      .getNowPlaying()
      .then((resp) => resp.json['subsonic-response'])
      .then((data) => {
        if (data.status === 'ok') {
          const nowPlayingEntries = data.nowPlaying?.entry || []
          const fetchTime = Date.now()
          setEntries(
            nowPlayingEntries.map((e) => ({ ...e, _fetchedAt: fetchTime })),
          )
          dispatch(nowPlayingCountSync({ count: nowPlayingEntries.length }))
        } else {
          throw new Error(
            data.error?.message || 'Failed to fetch now playing data',
          )
        }
      })
      .catch((error) => {
        notify('ra.page.error', 'warning', {
          messageArgs: { error: error.message || 'Unknown error' },
        })
      })
  const fetchList = useCallback(() => {
    if (fetchTimerRef.current) clearTimeout(fetchTimerRef.current)
    fetchTimerRef.current = setTimeout(() => {
      fetchTimerRef.current = null
      doFetchRef.current()
    }, 300)
  }, [])

  useEffect(() => {
    return () => {
      if (fetchTimerRef.current) clearTimeout(fetchTimerRef.current)
    }
  }, [])

  // Initialize count and entries on mount, and refresh on server/stream changes
  useEffect(() => {
    if (serverUp) fetchList()
  }, [fetchList, serverUp, streamReconnected])

  // Refresh immediately when NowPlaying updates from SSE events or panel is opened
  useEffect(() => {
    if (open && serverUp) {
      if (doFetchRef.current) doFetchRef.current()
    }
  }, [lastUpdate, open, serverUp])

  // Update current time every second when open to animate progress bars
  useInterval(() => setNow(Date.now()), open ? 1000 : null)

  // Periodic refresh when panel is open (10 seconds)
  useInterval(
    () => {
      if (open && serverUp) fetchList()
    },
    open ? 10000 : null,
  )

  // Periodic refresh when panel is closed (60 seconds) to keep badge accurate
  useInterval(
    () => {
      if (!open && serverUp) fetchList()
    },
    !open ? 60000 : null,
  )

  return (
    <div style={{ display: 'inline-flex' }}>
      <NowPlayingButton count={count} onClick={handleToggle} />
      <NowPlayingList
        anchorEl={anchorEl}
        open={open}
        onClose={handleMenuClose}
        entries={entries}
        deviceNames={deviceNames}
        currentVolume={currentVolume}
        outputDevice={outputDevice}
        now={now}
        onLinkClick={handleLinkClick}
        getArtistLink={getArtistLink}
        onTakeover={handleTakeover}
      />
    </div>
  )
}

NowPlayingPanel.propTypes = {}

export default NowPlayingPanel
