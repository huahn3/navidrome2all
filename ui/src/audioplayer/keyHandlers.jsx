const keyHandlers = (audioInstance, playerState, adjustVolume) => {
  const nextSong = () => {
    const idx = playerState.queue.findIndex(
      (item) => item.uuid === playerState.current.uuid,
    )
    return idx !== null ? playerState.queue[idx + 1] : null
  }

  const prevSong = () => {
    const idx = playerState.queue.findIndex(
      (item) => item.uuid === playerState.current.uuid,
    )
    return idx !== null ? playerState.queue[idx - 1] : null
  }

  return {
    TOGGLE_PLAY: (e) => {
      e.preventDefault()
      audioInstance && audioInstance.togglePlay()
    },
    // Volume goes through the store: <Player /> owns the <audio> element level
    // and re-asserts the stored value, so writing audioInstance.volume here
    // would be undone instantly and never reach a remote output.
    VOL_UP: () => adjustVolume(0.1),
    VOL_DOWN: () => adjustVolume(-0.1),
    PREV_SONG: (e) => {
      if (!e.metaKey && prevSong()) audioInstance && audioInstance.playPrev()
    },
    CURRENT_SONG: () => {
      window.location.href = `#/album/${playerState.current?.song.albumId}/show`
    },
    NEXT_SONG: (e) => {
      if (!e.metaKey && nextSong()) audioInstance && audioInstance.playNext()
    },
  }
}

export default keyHandlers
