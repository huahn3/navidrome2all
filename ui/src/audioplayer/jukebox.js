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

// fetchJson rejects with an HttpError carrying {status, body}. The body is
// whatever the server sent (plain text from http.Error, JSON from RespondWithError),
// so flatten it to something a human can read in a list of per-device failures.
const describeRequestError = (e) => {
  const status = e?.status
  const body = e?.body
  const detail =
    typeof body === 'string'
      ? body
      : body?.error || body?.message || e?.message || ''
  const text = String(detail).trim()
  if (!text) {
    return status ? `HTTP ${status}` : '请求失败'
  }
  return status ? `HTTP ${status}: ${text}` : text
}

export const listJukeboxOutputs = () =>
  httpClient('/api/jukebox/outputs').then((response) => response.json)

export const createJukeboxOutput = (device) =>
  postJSON('/api/jukebox/outputs', device)

export { describeRequestError }

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

// MPD has no discovery protocol, so the server probes the local /24 on its
// port (6600 by default). Admin-only, and it opens up to 254 short connections.
export const discoverMPD = ({ timeout = 4, port } = {}) => {
  const params = [`timeout=${timeout}`]
  if (port) {
    params.push(`port=${port}`)
  }
  return httpClient(`/api/jukebox/discover/mpd?${params.join('&')}`).then(
    (response) => response.json || [],
  )
}

// Performs a real MPD login so the form can validate before saving. The
// password only travels to our own server.
export const verifyMPD = (address, password) => {
  const headers = new Headers({ 'Content-Type': 'application/json' })
  return httpClient('/api/jukebox/verify/mpd', {
    method: 'POST',
    headers,
    body: JSON.stringify({ address, password: password || '' }),
  }).then((response) => response.json)
}

export const status = () =>
  httpClient('/api/jukebox/status').then((response) => response.json)

export const initXiaomiQR = () =>
  httpClient('/api/jukebox/outputs/xiaomi/qr/init').then(
    (response) => response.json,
  )

export const pollXiaomiQR = (lp) =>
  postJSON('/api/jukebox/outputs/xiaomi/qr/poll', { lp }).then(
    (response) => response.json,
  )

export const loginXiaomiPassword = (account, password) =>
  postJSON('/api/jukebox/outputs/xiaomi/login/password', {
    account,
    password,
  }).then((response) => response.json)

export const loginXiaomiPassToken = (userId, passToken) =>
  postJSON('/api/jukebox/outputs/xiaomi/login/passtoken', {
    userId,
    passToken,
  }).then((response) => response.json)
