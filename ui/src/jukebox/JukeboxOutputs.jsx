// Output device management, deliberately NOT built from react-admin's
// <List>/<SimpleForm>: the admin screens of Navidrome are dense tables, while
// this one is a device console where each output is a card you act on. It talks
// to the same endpoints the player uses (see ui/src/audioplayer/jukebox.js).
import React, { useCallback, useEffect, useState } from 'react'
import { useDispatch } from 'react-redux'
import { Title, useNotify, useTranslate } from 'react-admin'
import {
  Button,
  CardContent,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Fab,
  IconButton,
  Tooltip,
  Typography,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import {
  MdAdd,
  MdDelete,
  MdEdit,
  MdRefresh,
  MdSpeaker,
  MdCastConnected,
} from 'react-icons/md'
import httpClient from '../dataProvider/httpClient'
import { BROWSER_DEVICE, setOutputDevice } from '../actions'
import OutputEditorDialog from './OutputEditorDialog'
import { EMPTY_OUTPUT } from './outputConstants'

const useStyles = makeStyles((theme) => ({
  root: {
    marginTop: theme.spacing(2),
    marginBottom: theme.spacing(4),
    maxWidth: 980,
    marginLeft: 'auto',
    marginRight: 'auto',
    borderRadius: theme.shape.borderRadius * 2,
    boxShadow: theme.shadows[2],
  },
  header: {
    padding: theme.spacing(3),
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
    background:
      theme.palette.type === 'dark'
        ? 'rgba(255, 255, 255, 0.03)'
        : 'rgba(0, 0, 0, 0.02)',
  },
  headerIcon: {
    width: 40,
    height: 40,
    borderRadius: '50%',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: theme.palette.primary.main,
    color: theme.palette.primary.contrastText,
    fontSize: 22,
    flexShrink: 0,
  },
  headerText: { flex: 1, minWidth: 0 },
  title: {
    fontWeight: 700,
    fontSize: '1.25rem',
    color: theme.palette.text.primary,
  },
  subtitle: {
    fontSize: '0.85rem',
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(0.5),
  },
  content: { padding: theme.spacing(3) },
  headerAction: {
    flexShrink: 0,
    '@media screen and (max-width: 600px)': { display: 'none' },
  },
  // On phones the header button is hidden; this floating action button takes over
  // so adding a device stays reachable without scrolling.
  fab: {
    position: 'fixed',
    right: theme.spacing(3),
    bottom: theme.spacing(3),
    zIndex: theme.zIndex.speedDial,
    '@media screen and (min-width: 601px)': { display: 'none' },
  },
  sectionTitle: {
    fontWeight: 600,
    fontSize: '0.95rem',
    color: theme.palette.text.primary,
    marginBottom: theme.spacing(0.5),
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
  },
  sectionHint: {
    fontSize: '0.8rem',
    color: theme.palette.text.secondary,
    marginBottom: theme.spacing(2),
    lineHeight: 1.6,
  },
  // The "browser" card is the local output, so it is presented as a wide row
  // rather than a device card.
  localRow: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1.5),
    padding: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
    background: theme.palette.action.hover,
    flexWrap: 'wrap',
  },
  localRowActive: {
    borderColor: theme.palette.primary.main,
    background:
      theme.palette.type === 'dark'
        ? 'rgba(33, 150, 243, 0.12)'
        : 'rgba(33, 150, 243, 0.06)',
  },
  localInfo: { flex: 1, minWidth: 160 },
  localName: { fontWeight: 600 },
  localHint: { fontSize: '0.78rem', color: theme.palette.text.secondary },
  grid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))',
    gap: theme.spacing(2),
    marginTop: theme.spacing(2),
  },
  card: {
    padding: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
    background: theme.palette.background.paper,
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(1),
    transition: theme.transitions.create('box-shadow', {
      duration: theme.transitions.duration.short,
    }),
    '&:hover': { boxShadow: theme.shadows[2] },
  },
  cardActive: {
    borderColor: theme.palette.primary.main,
    boxShadow: `0 0 0 1px ${theme.palette.primary.main}`,
  },
  cardTop: { display: 'flex', alignItems: 'flex-start', gap: theme.spacing(1) },
  cardIcon: {
    width: 36,
    height: 36,
    borderRadius: '10px',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: 20,
    flexShrink: 0,
    background:
      theme.palette.type === 'dark'
        ? 'rgba(255, 255, 255, 0.06)'
        : 'rgba(0, 0, 0, 0.04)',
    color: theme.palette.text.secondary,
  },
  cardIconActive: {
    backgroundColor: theme.palette.primary.main,
    color: theme.palette.primary.contrastText,
  },
  cardText: { flex: 1, minWidth: 0 },
  cardName: {
    fontWeight: 600,
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  cardAddress: {
    fontSize: '0.78rem',
    color: theme.palette.text.secondary,
    wordBreak: 'break-word',
  },
  // A DLNA device is configured with its description URL, whose UDN path is long
  // and noisy. Show host:port and keep the full value in the tooltip.
  cardAddressFull: {
    fontSize: '0.72rem',
    color: theme.palette.text.hint || theme.palette.text.secondary,
    opacity: 0.75,
    marginTop: 2,
    wordBreak: 'break-all',
  },
  cardChips: { display: 'flex', gap: theme.spacing(0.75), flexWrap: 'wrap' },
  cardActions: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.5),
    marginTop: 'auto',
    paddingTop: theme.spacing(0.5),
  },
  grow: { flex: 1 },
  footerAdd: {
    '@media screen and (max-width: 600px)': { display: 'none' },
  },
  empty: {
    padding: theme.spacing(4),
    textAlign: 'center',
    color: theme.palette.text.secondary,
    fontSize: '0.88rem',
  },
  error: {
    padding: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
    backgroundColor:
      theme.palette.type === 'dark' ? 'rgba(244, 67, 54, 0.15)' : '#ffebee',
    color: theme.palette.type === 'dark' ? '#e57373' : '#c62828',
    fontSize: '0.85rem',
    marginBottom: theme.spacing(2),
  },
  loading: {
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    minHeight: 240,
  },
  footer: {
    display: 'flex',
    gap: theme.spacing(1.5),
    marginTop: theme.spacing(3),
    paddingTop: theme.spacing(2),
    borderTop: `1px solid ${theme.palette.divider}`,
    flexWrap: 'wrap',
  },
}))

const TYPE_META = {
  xiaomi: { label: '小爱音箱', icon: MdSpeaker, color: '#ff6900' },
  mpd: { label: 'MPD', icon: MdCastConnected, color: '#3f51b5' },
  dlna: { label: 'DLNA / UPnP', icon: MdCastConnected, color: '#00897b' },
}

// A DLNA output is addressed by its device description URL, e.g.
// http://192.168.1.10:8200/e522dfd8-....xml. The UDN path is long and adds
// nothing on the card, so show host:port and keep the full URL in the tooltip
// (and in the edit form, which always gets the unmasked value).
const formatAddress = (address, type) => {
  if (!address) return ''
  if ((type || '').toLowerCase() !== 'dlna') return address
  const withScheme = /^https?:\/\//i.test(address)
    ? address
    : `http://${address}`
  try {
    return new URL(withScheme).host
  } catch {
    return address
  }
}

const JukeboxOutputs = () => {
  const classes = useStyles()
  const translate = useTranslate()
  const notify = useNotify()
  const dispatch = useDispatch()

  const [outputs, setOutputs] = useState([])
  const [selected, setSelected] = useState(BROWSER_DEVICE)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [editor, setEditor] = useState(null) // { mode: 'create'|'edit', output }
  const [removing, setRemoving] = useState(null)

  const load = useCallback(async () => {
    setError(null)
    try {
      const [list, devices] = await Promise.all([
        httpClient('/api/jukebox/outputs').then((r) => r.json),
        httpClient('/api/jukebox/devices').then((r) => r.json),
      ])
      const current = devices?.selected || BROWSER_DEVICE
      setOutputs(Array.isArray(list) ? list : [])
      setSelected(current)
      // The player dock renders from the store, so mirror the server selection
      // there: otherwise "set as output" here would not move the dock's picker.
      dispatch(setOutputDevice(current))
    } catch (e) {
      setError(e.message || 'failed to load outputs')
    } finally {
      setLoading(false)
    }
  }, [dispatch])

  useEffect(() => {
    load()
  }, [load])

  const select = useCallback(
    async (deviceId) => {
      const id = deviceId || BROWSER_DEVICE
      try {
        await httpClient('/api/jukebox/select', {
          method: 'POST',
          headers: new Headers({ 'Content-Type': 'application/json' }),
          body: JSON.stringify({ device_id: id }),
        })
        setSelected(id)
        dispatch(setOutputDevice(id))
        notify('resources.jukeboxOutput.messages.selected', {
          type: 'info',
          messageArgs: {
            name:
              id === BROWSER_DEVICE
                ? translate('resources.jukeboxOutput.localOutput', {
                    _: '本机浏览器',
                  })
                : outputs.find((o) => o.id === id)?.name || id,
          },
        })
      } catch (e) {
        notify('ra.page.error', {
          type: 'warning',
          messageArgs: { error: e.message },
        })
      }
    },
    [dispatch, notify, outputs, translate],
  )

  const remove = useCallback(async () => {
    const target = removing
    setRemoving(null)
    if (!target) return
    try {
      await httpClient(
        `/api/jukebox/outputs/${encodeURIComponent(target.id)}`,
        {
          method: 'DELETE',
        },
      )
      notify('resources.jukeboxOutput.messages.deleted', {
        type: 'info',
        messageOptions: {
          defaultMessage: '输出设备已删除',
        },
      })
      load()
    } catch (e) {
      notify('ra.page.error', {
        type: 'warning',
        messageArgs: { error: e.message },
      })
    }
  }, [load, notify, removing])

  const typeMeta = (type) =>
    TYPE_META[(type || '').toLowerCase()] || {
      label: (type || '?').toUpperCase(),
      icon: MdSpeaker,
      color: '#9e9e9e',
    }

  const renderChip = (label, color, filled) => (
    <Chip
      key={label}
      size="small"
      label={label}
      style={{
        backgroundColor: filled ? color : 'transparent',
        color: filled ? '#fff' : color,
        border: `1px solid ${color}`,
        height: 22,
        fontWeight: 600,
      }}
    />
  )

  return (
    <div className={classes.root}>
      <Title title="输出设备" />
      <div className={classes.header}>
        <div className={classes.headerIcon}>
          <MdSpeaker />
        </div>
        <div className={classes.headerText}>
          <div className={classes.title}>
            {translate('menu.jukebox.name', { _: '输出设备' })}
          </div>
          <div className={classes.subtitle}>
            {translate('menu.jukebox.subtitle', {
              _: '把播放交给局域网里的音箱或 NAS。这里选择哪个输出，播放器和「正在播放」里的其他设备都会跟着切过去。',
            })}
          </div>
        </div>
        <Button
          variant="contained"
          color="primary"
          disableElevation
          startIcon={<MdAdd />}
          onClick={() => setEditor({ mode: 'create', output: EMPTY_OUTPUT })}
          className={classes.headerAction}
        >
          {translate('resources.jukeboxOutput.actions.add', {
            _: '新增输出设备',
          })}
        </Button>
        <Tooltip title={translate('ra.action.refresh', { _: '刷新' })}>
          <span>
            <IconButton
              onClick={load}
              disabled={loading}
              aria-label={translate('ra.action.refresh', { _: '刷新' })}
              size="small"
            >
              <MdRefresh />
            </IconButton>
          </span>
        </Tooltip>
      </div>

      <CardContent className={classes.content}>
        {error && <div className={classes.error}>{error}</div>}

        <div className={classes.sectionTitle}>
          <MdSpeaker fontSize="small" />
          {translate('resources.jukeboxOutput.sections.current', {
            _: '当前输出',
          })}
        </div>
        <div className={classes.sectionHint}>
          {translate('resources.jukeboxOutput.sections.currentHint', {
            _: '「本机」= 这台设备自己出声；选其它设备时，本机播放器会静音并把进度交给它。',
          })}
        </div>

        <div
          className={`${classes.localRow} ${
            selected === BROWSER_DEVICE ? classes.localRowActive : ''
          }`}
        >
          <div
            className={`${classes.cardIcon} ${
              selected === BROWSER_DEVICE ? classes.cardIconActive : ''
            }`}
          >
            <MdCastConnected />
          </div>
          <div className={classes.localInfo}>
            <div className={classes.localName}>
              {translate('resources.jukeboxOutput.localOutput', {
                _: '本机浏览器',
              })}
            </div>
            <div className={classes.localHint}>
              {translate('resources.jukeboxOutput.localOutputHint', {
                _: '在这台设备上直接播放',
              })}
            </div>
          </div>
          {selected === BROWSER_DEVICE ? (
            renderChip(
              translate('resources.jukeboxOutput.currentBadge', {
                _: '使用中',
              }),
              '#2e7d32',
              true,
            )
          ) : (
            <Button
              size="small"
              variant="outlined"
              onClick={() => select(BROWSER_DEVICE)}
            >
              {translate('resources.jukeboxOutput.actions.use', {
                _: '切到这里',
              })}
            </Button>
          )}
        </div>

        <Divider />

        <div className={classes.sectionTitle} style={{ marginTop: 20 }}>
          <MdCastConnected fontSize="small" />
          {translate('resources.jukeboxOutput.sections.devices', {
            _: '局域网设备',
          })}
        </div>
        <div className={classes.sectionHint}>
          {translate('resources.jukeboxOutput.sections.devicesHint', {
            _: '每张卡片是一台可出声的设备：可以设为当前输出、修改连接信息或删除。配置文件里定义的设备只读。',
          })}
        </div>

        {loading ? (
          <div className={classes.loading}>
            <CircularProgress size={28} />
          </div>
        ) : outputs.length === 0 ? (
          <div className={classes.empty}>
            {translate('resources.jukeboxOutput.empty', {
              _: '还没有配置任何输出设备。',
            })}
          </div>
        ) : (
          <div className={classes.grid}>
            {outputs.map((output) => {
              const meta = typeMeta(output.type)
              const Icon = meta.icon
              const isActive = selected === output.id
              const readOnly = output.source === 'config'
              return (
                <div
                  key={output.id}
                  className={`${classes.card} ${
                    isActive ? classes.cardActive : ''
                  }`}
                >
                  <div className={classes.cardTop}>
                    <div
                      className={`${classes.cardIcon} ${
                        isActive ? classes.cardIconActive : ''
                      }`}
                      style={
                        isActive
                          ? { backgroundColor: meta.color, color: '#fff' }
                          : { color: meta.color }
                      }
                    >
                      <Icon />
                    </div>
                    <div className={classes.cardText}>
                      <div className={classes.cardName}>{output.name}</div>
                      <div className={classes.cardAddress}>
                        {formatAddress(output.address, output.type)}
                      </div>
                      {formatAddress(output.address, output.type) !==
                        output.address && (
                        <Tooltip title={output.address}>
                          <div className={classes.cardAddressFull}>
                            {translate(
                              'resources.jukeboxOutput.hint.descriptionUrl',
                              { _: '设备描述 URL（点编辑可看完整地址）' },
                            )}
                          </div>
                        </Tooltip>
                      )}
                    </div>
                  </div>

                  <div className={classes.cardChips}>
                    {renderChip(meta.label, meta.color, false)}
                    {readOnly &&
                      renderChip(
                        translate(
                          'resources.jukeboxOutput.sourceValues.config',
                          {
                            _: '配置文件',
                          },
                        ),
                        '#78909c',
                        false,
                      )}
                  </div>

                  <div className={classes.cardActions}>
                    {isActive ? (
                      renderChip(
                        translate('resources.jukeboxOutput.currentBadge', {
                          _: '使用中',
                        }),
                        '#2e7d32',
                        true,
                      )
                    ) : (
                      <Button
                        size="small"
                        variant="contained"
                        color="primary"
                        disableElevation
                        onClick={() => select(output.id)}
                      >
                        {translate('resources.jukeboxOutput.actions.use', {
                          _: '设为输出',
                        })}
                      </Button>
                    )}
                    <div className={classes.grow} />
                    <Tooltip
                      title={translate('resources.jukeboxOutput.actions.edit', {
                        _: '编辑',
                      })}
                    >
                      <IconButton
                        size="small"
                        aria-label={translate(
                          'resources.jukeboxOutput.actions.edit',
                          { _: '编辑' },
                        )}
                        onClick={() => setEditor({ mode: 'edit', output })}
                      >
                        <MdEdit fontSize="small" />
                      </IconButton>
                    </Tooltip>
                    {!readOnly && (
                      <Tooltip
                        title={translate(
                          'resources.jukeboxOutput.actions.delete',
                          {
                            _: '删除',
                          },
                        )}
                      >
                        <IconButton
                          size="small"
                          aria-label={translate(
                            'resources.jukeboxOutput.actions.delete',
                            { _: '删除' },
                          )}
                          onClick={() => setRemoving(output)}
                        >
                          <MdDelete fontSize="small" />
                        </IconButton>
                      </Tooltip>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        )}

        <div className={classes.footer}>
          <Button
            variant="outlined"
            color="primary"
            startIcon={<MdAdd />}
            onClick={() => setEditor({ mode: 'create', output: EMPTY_OUTPUT })}
            className={classes.footerAdd}
          >
            {translate('resources.jukeboxOutput.actions.add', {
              _: '新增输出设备',
            })}
          </Button>
        </div>
      </CardContent>

      {/* Phones hide the header button, so keep adding a device one tap away. */}
      <div className={classes.fab}>
        <Fab
          color="primary"
          aria-label={translate('resources.jukeboxOutput.actions.add', {
            _: '新增输出设备',
          })}
          onClick={() => setEditor({ mode: 'create', output: EMPTY_OUTPUT })}
        >
          <MdAdd />
        </Fab>
      </div>

      {editor && (
        <OutputEditorDialog
          mode={editor.mode}
          output={editor.output}
          onClose={() => setEditor(null)}
          onSaved={({ keepOpen } = {}) => {
            // 批量添加部分失败时留在弹窗里，让用户能读到失败原因
            load()
            if (!keepOpen) {
              setEditor(null)
            }
          }}
        />
      )}

      <Dialog
        open={!!removing}
        onClose={() => setRemoving(null)}
        maxWidth="xs"
        fullWidth
      >
        <DialogTitle>
          {translate('resources.jukeboxOutput.deleteTitle', {
            _: '删除输出设备',
          })}
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2">
            {translate('resources.jukeboxOutput.deleteConfirm', {
              _: '确定要删除「{{name}}」吗？此操作不可撤销。',
              name: removing?.name,
            })}
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setRemoving(null)}>
            {translate('ra.action.cancel', { _: '取消' })}
          </Button>
          <Button onClick={remove} color="secondary" variant="contained">
            {translate('ra.action.delete', { _: '删除' })}
          </Button>
        </DialogActions>
      </Dialog>
    </div>
  )
}

export default JukeboxOutputs
