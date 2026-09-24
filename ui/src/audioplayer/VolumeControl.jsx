import React, { useCallback, useRef } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { useTranslate } from 'react-admin'
import IconButton from '@material-ui/core/IconButton'
import Slider from '@material-ui/core/Slider'
import { makeStyles } from '@material-ui/core/styles'
import {
  RiVolumeDownLine,
  RiVolumeMuteLine,
  RiVolumeUpLine,
} from 'react-icons/ri'
import { setVolume } from '../actions'

const useStyles = makeStyles((theme) => ({
  root: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.5),
    minWidth: 0,
  },
  slider: {
    width: 110,
    flex: '0 0 auto',
  },
  compact: {
    width: 'auto',
    flex: '1 1 auto',
    minWidth: 0,
  },
  percent: {
    fontSize: '0.75rem',
    fontVariantNumeric: 'tabular-nums',
    opacity: 0.8,
    textAlign: 'right',
    width: '2.5rem',
  },
  icon: {
    fontSize: (props) => (props.compact ? '20px' : '24px'),
  },
  rail: {
    backgroundColor:
      theme.palette.type === 'dark'
        ? 'rgba(255,255,255,0.35)'
        : 'rgba(0,0,0,0.2)',
    opacity: 1,
    height: 4,
  },
  track: {
    backgroundColor: theme.palette.primary.main,
    height: 4,
  },
  thumb: {
    width: 14,
    height: 14,
    marginTop: -5,
    marginLeft: -7,
    backgroundColor: theme.palette.common.white,
    border: `2px solid ${theme.palette.primary.main}`,
    '&::before': { display: 'none' },
  },
}))

// VolumeControl is the single volume UI for both the browser output and remote
// outputs. It only reads/writes the store; Player.jsx applies the value to the
// <audio> element and to the selected device.
const VolumeControl = ({ compact = false }) => {
  const classes = useStyles({ compact })
  const dispatch = useDispatch()
  const translate = useTranslate()
  const storedVolume = useSelector((state) => state.player.volume)
  const lastPercentRef = useRef(50)
  const percent = Math.round((storedVolume ?? 1) * 100)

  const handleChange = useCallback(
    (_, value) => {
      dispatch(setVolume(value / 100))
    },
    [dispatch],
  )

  const handleToggleMute = useCallback(
    (e) => {
      e.stopPropagation()
      if (percent > 0) {
        lastPercentRef.current = percent
        dispatch(setVolume(0))
      } else {
        dispatch(setVolume(lastPercentRef.current / 100))
      }
    },
    [dispatch, percent],
  )

  const Icon =
    percent === 0
      ? RiVolumeMuteLine
      : percent < 50
        ? RiVolumeDownLine
        : RiVolumeUpLine

  return (
    <div
      className={classes.root}
      data-testid="volume-control"
      onClick={(e) => e.stopPropagation()}
      onMouseDown={(e) => e.stopPropagation()}
      onTouchStart={(e) => e.stopPropagation()}
    >
      <IconButton
        size={compact ? undefined : 'small'}
        onClick={handleToggleMute}
        aria-label={translate('player.volumeText')}
        title={translate('player.volumeText')}
      >
        <Icon className={classes.icon} />
      </IconButton>
      <Slider
        className={[classes.slider, compact ? classes.compact : null]
          .filter(Boolean)
          .join(' ')}
        classes={{
          rail: classes.rail,
          track: classes.track,
          thumb: classes.thumb,
        }}
        value={percent}
        min={0}
        max={100}
        step={1}
        onChange={handleChange}
        aria-label={translate('player.volumeText')}
        valueLabelDisplay="auto"
      />
      <span className={classes.percent}>{percent}%</span>
    </div>
  )
}

export default VolumeControl
