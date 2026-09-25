import { v4 as uuidv4 } from 'uuid'
import subsonic from '../subsonic'
import { decisionService } from '../transcode'
import {
  BROWSER_DEVICE,
  PLAYER_ADD_TRACKS,
  PLAYER_CLEAR_QUEUE,
  PLAYER_CURRENT,
  PLAYER_PLAY_NEXT,
  PLAYER_PLAY_TRACKS,
  PLAYER_SET_OUTPUT_DEVICE,
  PLAYER_SET_TRACK,
  PLAYER_SET_VOLUME,
  PLAYER_SYNC_QUEUE,
  PLAYER_SET_MODE,
  PLAYER_REFRESH_QUEUE,
  PLAYER_UPDATE_SONG_LYRIC,
} from '../actions'
import config from '../config'

const initialState = {
  queue: [],
  current: {},
  clear: false,
  volume: config.defaultUIVolume / 100,
  savedPlayIndex: 0,
  outputDevice: BROWSER_DEVICE,
  bilingualActive: false,
  originalLyrics: {},
  bilingualLyrics: {},
}

const pad = (value) => {
  const str = value.toString()
  if (str.length === 1) {
    return `0${str}`
  } else {
    return str
  }
}

const makeMusicSrc = (trackId) =>
  decisionService.getProfile()
    ? () =>
        decisionService
          .resolveStreamUrl(trackId)
          .catch(() => subsonic.streamUrl(trackId))
    : subsonic.streamUrl(trackId)

const mapToAudioLists = (item) => {
  // If item comes from a playlist, trackId is mediaFileId
  const trackId = item.mediaFileId || item.id

  if (item.isRadio) {
    return {
      trackId,
      uuid: uuidv4(),
      name: item.name,
      song: item,
      musicSrc: item.streamUrl,
      cover: item.cover,
      isRadio: true,
    }
  }

  const { lyrics } = item
  let lyricText = ''

  if (lyrics) {
    const structured = JSON.parse(lyrics)
    for (const structuredLyric of structured) {
      if (structuredLyric.synced) {
        for (const line of structuredLyric.line) {
          let time = Math.floor(line.start / 10)
          const ms = time % 100
          time = Math.floor(time / 100)
          const sec = time % 60
          time = Math.floor(time / 60)
          const min = time % 60

          ms.toString()
          lyricText += `[${pad(min)}:${pad(sec)}.${pad(ms)}] ${line.value}\n`
        }
      }
    }
  }

  return {
    trackId,
    uuid: uuidv4(),
    song: item,
    name: item.title,
    lyric: lyricText,
    singer: item.artist,
    duration: item.duration,
    musicSrc: makeMusicSrc(trackId),
    cover: subsonic.getCoverArtUrl(
      {
        id: trackId,
        updatedAt: item.updatedAt,
        album: item.album,
      },
      300,
    ),
  }
}

const reduceClearQueue = (previousState) => ({
  ...initialState,
  clear: true,
  // The selected sound output is a device preference, keep it across queue clears
  outputDevice: previousState.outputDevice || BROWSER_DEVICE,
  originalLyrics: previousState.originalLyrics || {},
  bilingualLyrics: previousState.bilingualLyrics || {},
})

const reducePlayTracks = (state, { data, id }) => {
  let playIndex = 0
  const originalLyrics = { ...state.originalLyrics }
  const queue = Object.keys(data).map((key, idx) => {
    if (key === id) {
      playIndex = idx
    }
    const item = mapToAudioLists(data[key])
    if (item.trackId && item.lyric) {
      originalLyrics[item.trackId] = item.lyric
    }
    return item
  })
  return {
    ...state,
    queue,
    playIndex,
    clear: true,
    bilingualActive: false,
    originalLyrics,
  }
}

const reduceSetTrack = (state, { data }) => {
  const item = mapToAudioLists(data)
  const originalLyrics = { ...state.originalLyrics }
  if (item.trackId && item.lyric) {
    originalLyrics[item.trackId] = item.lyric
  }
  return {
    ...state,
    queue: [item],
    playIndex: 0,
    clear: true,
    bilingualActive: false,
    originalLyrics,
  }
}

const reduceAddTracks = (state, { data }) => {
  const queue = [...state.queue]
  const originalLyrics = { ...state.originalLyrics }
  Object.keys(data).forEach((id) => {
    const item = mapToAudioLists(data[id])
    if (item.trackId && item.lyric) {
      originalLyrics[item.trackId] = item.lyric
    }
    queue.push(item)
  })
  return { ...state, queue, clear: false, originalLyrics }
}

const reducePlayNext = (state, { data }) => {
  const newTracks = Object.keys(data).map((id) => mapToAudioLists(data[id]))
  const newQueue = []
  const current = state.current || {}
  let foundPos = false
  state.queue.forEach((item) => {
    newQueue.push(item)
    if (item.uuid === current.uuid) {
      foundPos = true
      newQueue.push(...newTracks)
    }
  })
  if (!foundPos) {
    newQueue.push(...newTracks)
  }

  return {
    ...state,
    queue: newQueue,
    clear: true,
  }
}

const reduceSetVolume = (state, { data: { volume } }) => {
  return {
    ...state,
    volume,
  }
}

const reduceSyncQueue = (state, { data: { audioInfo, audioLists } }) => {
  // Keep clear and playIndex alive when there is a pending track switch.
  // A switch is pending when playIndex is set AND either:
  //   - playIndex differs from savedPlayIndex, OR
  //   - clear is true (a new queue was loaded, e.g. after clearQueue + playTracks)
  // The clear check handles the edge case where both playIndex and
  // savedPlayIndex are 0 (close player then play a new album from track 1).
  const hasPendingSwitch =
    state.playIndex != null &&
    (state.clear || state.playIndex !== state.savedPlayIndex)

  // Merge audioLists while preserving any updated lyrics from current state.queue
  let hasModifiedLyric = false
  const mergedQueue = (audioLists || []).map((item) => {
    const existing = state.queue.find(
      (q) => q.trackId === item.trackId || q.uuid === item.uuid,
    )
    if (existing && existing.lyric && existing.lyric !== item.lyric) {
      hasModifiedLyric = true
      return { ...item, lyric: existing.lyric }
    }
    return item
  })

  return {
    ...state,
    queue: hasModifiedLyric ? mergedQueue : audioLists,
    clear: hasPendingSwitch ? state.clear : false,
    playIndex: hasPendingSwitch ? state.playIndex : undefined,
  }
}

const reduceCurrent = (state, { data }) => {
  const current = data.ended ? {} : { ...data }
  const currentTrackId = current.trackId || (current.song && current.song.id)

  // Ensure current always retains any updated lyric for the track
  const existingInQueue = state.queue.find(
    (item) => item.trackId === currentTrackId,
  )
  if (existingInQueue && existingInQueue.lyric) {
    current.lyric = existingInQueue.lyric
  }

  const originalLyrics = { ...state.originalLyrics }
  if (
    currentTrackId &&
    current.lyric &&
    !originalLyrics[currentTrackId] &&
    !state.bilingualActive
  ) {
    originalLyrics[currentTrackId] = current.lyric
  }

  const savedPlayIndex = state.queue.findIndex(
    (item) => item.uuid === current.uuid,
  )
  const isNewTrack =
    state.current?.trackId &&
    currentTrackId &&
    state.current.trackId !== currentTrackId
  // When a track selection is pending (playIndex is set), keep it alive
  // until the music player confirms it actually switched to the requested
  // track. Without this, a premature onAudioPlay callback for the
  // still-playing old track would overwrite the pending selection.
  const pending = state.playIndex != null && savedPlayIndex !== state.playIndex
  return {
    ...state,
    current,
    bilingualActive: isNewTrack ? false : state.bilingualActive,
    originalLyrics,
    playIndex: pending ? state.playIndex : undefined,
    clear: pending ? state.clear : false,
    savedPlayIndex: pending ? state.savedPlayIndex : savedPlayIndex,
  }
}

const reduceMode = (state, { data: { mode } }) => {
  return {
    ...state,
    mode,
  }
}

const reduceOutputDevice = (state, { data: { deviceId } }) => {
  return {
    ...state,
    outputDevice: deviceId || BROWSER_DEVICE,
  }
}

const reduceUpdateSongLyric = (
  state,
  { data: { trackId, lyric, isBilingual } },
) => {
  const originalLyrics = { ...state.originalLyrics }
  const bilingualLyrics = { ...state.bilingualLyrics }

  if (isBilingual) {
    bilingualLyrics[trackId] = lyric
  } else {
    originalLyrics[trackId] = lyric
  }

  const queue = state.queue.map((item) => {
    if (item.trackId === trackId) {
      return { ...item, lyric }
    }
    return item
  })
  const current =
    state.current && state.current.trackId === trackId
      ? { ...state.current, lyric }
      : state.current
  return {
    ...state,
    queue,
    current,
    bilingualActive: !!isBilingual,
    originalLyrics,
    bilingualLyrics,
  }
}

export const playerReducer = (previousState = initialState, payload) => {
  const { type } = payload
  switch (type) {
    case PLAYER_CLEAR_QUEUE:
      return reduceClearQueue(previousState)
    case PLAYER_PLAY_TRACKS:
      return reducePlayTracks(previousState, payload)
    case PLAYER_SET_TRACK:
      return reduceSetTrack(previousState, payload)
    case PLAYER_ADD_TRACKS:
      return reduceAddTracks(previousState, payload)
    case PLAYER_PLAY_NEXT:
      return reducePlayNext(previousState, payload)
    case PLAYER_SET_VOLUME:
      return reduceSetVolume(previousState, payload)
    case PLAYER_SYNC_QUEUE:
      return reduceSyncQueue(previousState, payload)
    case PLAYER_CURRENT:
      return reduceCurrent(previousState, payload)
    case PLAYER_SET_MODE:
      return reduceMode(previousState, payload)
    case PLAYER_SET_OUTPUT_DEVICE:
      return reduceOutputDevice(previousState, payload)
    case PLAYER_UPDATE_SONG_LYRIC:
      return reduceUpdateSongLyric(previousState, payload)
    case PLAYER_REFRESH_QUEUE: {
      const resolvedUrls = payload.data || {}
      return {
        ...previousState,
        queue: previousState.queue.map((item) => ({
          ...item,
          musicSrc: item.isRadio
            ? item.musicSrc
            : resolvedUrls[item.trackId] || subsonic.streamUrl(item.trackId),
        })),
        clear: true,
        autoPlay: false,
        playIndex:
          previousState.savedPlayIndex >= 0 ? previousState.savedPlayIndex : 0,
      }
    }
    default:
      return previousState
  }
}
