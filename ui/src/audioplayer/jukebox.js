import { httpClient } from '../dataProvider'
import { BROWSER_DEVICE } from '../actions'

// The server resets its selected output to the browser on restart. Keep track
// of the last selection we sent, so commands re-select it when needed.
let selectedOnServer = BROWSER_DEVICE

const postJSON = (url, payload) => {
  const headers = new Headers({ 'Content-Type': 'application/json' })
  return httpClient(url, {
    method: 'POST',
    headers,
    body: JSON.stringify(payload),
  })
}

export const fetchDevices = () =>
  httpClient('/api/jukebox/devices').then((response) => {
    const data = response.json
    if (data && data.selected) {
      selectedOnServer = data.selected
    }
    return data
  })

export const selectDevice = (deviceId) => {
  const device = deviceId || BROWSER_DEVICE
  return postJSON('/api/jukebox/select', { device_id: device }).then(
    (response) => {
      selectedOnServer = (response.json && response.json.selected) || device
      return response.json
    },
  )
}

const ensureSelected = (deviceId) =>
  selectedOnServer === deviceId ? Promise.resolve() : selectDevice(deviceId)

export const play = (deviceId, { songId, streamUrl, position } = {}) =>
  ensureSelected(deviceId).then(() =>
    postJSON('/api/jukebox/play', {
      song_id: songId,
      stream_url: streamUrl,
      position: position || 0,
    }),
  )

export const control = (deviceId, action, value) =>
  ensureSelected(deviceId).then(() =>
    postJSON('/api/jukebox/control', { action, value: value || 0 }),
  )

export const discoverRenderers = (timeout = 6) =>
  httpClient(`/api/jukebox/discover?timeout=${timeout}`).then(
    (response) => response.json || [],
  )

export const status = () =>
  httpClient('/api/jukebox/status').then((response) => response.json)
