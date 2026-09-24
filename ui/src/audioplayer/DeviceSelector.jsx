import React, { useCallback, useEffect, useState } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { useTranslate } from 'react-admin'
import IconButton from '@material-ui/core/IconButton'
import ListItemIcon from '@material-ui/core/ListItemIcon'
import ListItemText from '@material-ui/core/ListItemText'
import Menu from '@material-ui/core/Menu'
import MenuItem from '@material-ui/core/MenuItem'
import Tooltip from '@material-ui/core/Tooltip'
import { makeStyles } from '@material-ui/core/styles'
import { RiCheckLine, RiSpeaker2Line } from 'react-icons/ri'
import config from '../config'
import { BROWSER_DEVICE, setOutputDevice } from '../actions'
import * as jukebox from './jukebox'

const useStyles = makeStyles(() => ({
  menuItem: {
    minWidth: 180,
  },
  selectedItem: {
    fontWeight: 500,
  },
}))

// DeviceSelector renders the sound-output picker shown in the player toolbar.
// It only updates the selected device in the store; the actual playback
// hand-over is performed by <Player /> when the selection changes.
const DeviceSelector = ({
  isDesktop = true,
  buttonClassName,
  iconClassName,
}) => {
  const classes = useStyles()
  const dispatch = useDispatch()
  const translate = useTranslate()
  const selectedDevice =
    useSelector((state) => state.player.outputDevice) || BROWSER_DEVICE
  const [anchorEl, setAnchorEl] = useState(null)
  const [devices, setDevices] = useState(null)

  const loadDevices = useCallback(() => {
    jukebox
      .fetchDevices()
      .then((data) => setDevices((data && data.devices) || []))
      .catch(() => setDevices([]))
  }, [])

  useEffect(() => {
    if (!config.jukeboxEnabled) {
      return
    }
    loadDevices()
  }, [loadDevices])

  const handleOpen = useCallback(
    (event) => {
      event.stopPropagation()
      setAnchorEl(event.currentTarget)
      loadDevices()
    },
    [loadDevices],
  )

  const handleClose = useCallback(() => setAnchorEl(null), [])

  const handleSelect = useCallback(
    (deviceId) => (event) => {
      event.stopPropagation()
      handleClose()
      if (deviceId !== selectedDevice) {
        dispatch(setOutputDevice(deviceId))
      }
    },
    [dispatch, handleClose, selectedDevice],
  )

  if (!config.jukeboxEnabled) {
    return null
  }

  const deviceLabel = (device) =>
    device.type === 'builtin'
      ? translate('jukebox.browser')
      : device.name || device.id

  return (
    <>
      <Tooltip title={translate('jukebox.outputDevice')}>
        <IconButton
          size={isDesktop ? 'small' : undefined}
          onClick={handleOpen}
          onMouseDown={(e) => e.stopPropagation()}
          data-testid="device-selector-button"
          aria-label={translate('jukebox.outputDevice')}
          className={buttonClassName}
        >
          <RiSpeaker2Line className={iconClassName} />
        </IconButton>
      </Tooltip>
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleClose}
        getContentAnchorEl={null}
        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
        transformOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        {devices === null && <MenuItem disabled>...</MenuItem>}
        {devices !== null && devices.length === 0 && (
          <MenuItem disabled>{translate('jukebox.errorDevices')}</MenuItem>
        )}
        {devices !== null &&
          devices.map((device) => (
            <MenuItem
              key={device.id}
              selected={device.id === selectedDevice}
              onClick={handleSelect(device.id)}
              className={classes.menuItem}
            >
              <ListItemIcon>
                {device.id === selectedDevice ? <RiCheckLine /> : null}
              </ListItemIcon>
              <ListItemText
                primary={deviceLabel(device)}
                className={
                  device.id === selectedDevice
                    ? classes.selectedItem
                    : undefined
                }
              />
            </MenuItem>
          ))}
      </Menu>
    </>
  )
}

export default DeviceSelector
