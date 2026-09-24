import React, { useCallback } from 'react'
import { useDispatch } from 'react-redux'
import { useGetOne } from 'react-admin'
import { GlobalHotKeys } from 'react-hotkeys'
import IconButton from '@material-ui/core/IconButton'
import { useMediaQuery } from '@material-ui/core'
import { RiSaveLine } from 'react-icons/ri'
import { LoveButton, useToggleLove } from '../common'
import { openSaveQueueDialog } from '../actions'
import DeviceSelector from './DeviceSelector'
import VolumeControl from './VolumeControl'
import { keyMap } from '../hotkeys'
import { makeStyles } from '@material-ui/core/styles'

const useStyles = makeStyles((theme) => ({
  toolbar: {
    display: 'flex',
    alignItems: 'center',
    flexGrow: 1,
    justifyContent: 'flex-end',
    gap: '0.5rem',
    listStyle: 'none',
    padding: 0,
    margin: 0,
  },
  mobileVolumeRow: {
    display: 'inline-flex',
    alignItems: 'center',
    listStyle: 'none',
    padding: 0,
    margin: 0,
    height: 34,
    flex: '1 1 auto',
    minWidth: 0,
  },
  mobileListItem: {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    listStyle: 'none',
    padding: 0,
    margin: 0,
    height: 34,
    width: 34,
  },
  button: {
    width: '2.5rem',
    height: '2.5rem',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 0,
  },
  mobileButton: {
    width: 34,
    height: 34,
    minWidth: 34,
    maxWidth: 34,
    padding: 0,
    margin: 0,
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '50%',
    WebkitTapHighlightColor: 'transparent',
    outline: 'none',
    userSelect: 'none',
    WebkitUserSelect: 'none',
    '&:focus, &:focus-visible, &:active': {
      outline: 'none',
      boxShadow: 'none',
      WebkitTapHighlightColor: 'transparent',
    },
    '& svg': {
      width: 19,
      height: 19,
      fontSize: 19,
    },
  },
  mobileIcon: {
    fontSize: '19px',
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: 19,
    height: 19,
    minWidth: 19,
    maxWidth: 19,
    minHeight: 19,
    maxHeight: 19,
  },
}))

const PlayerToolbar = ({ id, isRadio }) => {
  const dispatch = useDispatch()
  const { data, loading } = useGetOne('song', id, { enabled: !!id && !isRadio })
  const [toggleLove, toggling] = useToggleLove('song', data)
  const isDesktop = useMediaQuery('(min-width:810px)')
  const classes = useStyles()

  const handlers = {
    TOGGLE_LOVE: useCallback(() => toggleLove(), [toggleLove]),
  }

  const handleSaveQueue = useCallback(
    (e) => {
      dispatch(openSaveQueueDialog())
      e.stopPropagation()
    },
    [dispatch],
  )

  const buttonClass = isDesktop ? classes.button : classes.mobileButton
  const listItemClass = isDesktop ? classes.toolbar : classes.mobileListItem

  const saveQueueButton = (
    <IconButton
      size={isDesktop ? 'small' : undefined}
      disableRipple={!isDesktop}
      onClick={handleSaveQueue}
      disabled={isRadio}
      data-testid="save-queue-button"
      className={buttonClass}
    >
      <RiSaveLine className={!isDesktop ? classes.mobileIcon : undefined} />
    </IconButton>
  )

  const loveButton = (
    <LoveButton
      record={data}
      resource={'song'}
      size={isDesktop ? undefined : 'inherit'}
      disabled={loading || toggling || !id || isRadio}
      className={buttonClass}
      disableRipple={!isDesktop}
    />
  )

  return (
    <>
      <GlobalHotKeys keyMap={keyMap} handlers={handlers} allowChanges />
      {isDesktop ? (
        <li className={`${listItemClass} item`}>
          {saveQueueButton}
          {loveButton}
          <DeviceSelector
            isDesktop={isDesktop}
            buttonClassName={buttonClass}
            iconClassName={!isDesktop ? classes.mobileIcon : undefined}
          />
          <VolumeControl />
        </li>
      ) : (
        <>
          <li
            className={`${classes.mobileVolumeRow} item`}
            data-testid="volume-row"
          >
            <VolumeControl compact />
          </li>
          <li className={`${listItemClass} item`}>{saveQueueButton}</li>
          <li className={`${listItemClass} item`}>{loveButton}</li>
          <li className={`${listItemClass} item`}>
            <DeviceSelector
              isDesktop={isDesktop}
              buttonClassName={buttonClass}
              iconClassName={!isDesktop ? classes.mobileIcon : undefined}
            />
          </li>
        </>
      )}
    </>
  )
}

export default PlayerToolbar
