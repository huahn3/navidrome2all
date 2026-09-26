// Create/edit dialog for one output device, built as a short wizard instead of
// one long form: the previous version showed every field of every type at once,
// which made the common case (pick a type, fill an address) hard to scan.
// Step 1 = choose the type on big cards, step 2 = only the fields that type
// needs, step 3 = the rarely used extras. It keeps react-final-form underneath
// (not react-admin's <SimpleForm>) so the layout matches this console and
// XiaomiAuthBlock still finds the form context it expects.
import React, { useCallback, useEffect, useRef, useState } from 'react'
import PropTypes from 'prop-types'
import { useNotify, useTranslate } from 'react-admin'
import { Field, Form, useField, useForm } from 'react-final-form'
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
  Typography,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import {
  MdCastConnected,
  MdCheck,
  MdSearch,
  MdSpeaker,
  MdWifi,
} from 'react-icons/md'
import httpClient from '../dataProvider/httpClient'
import {
  discoverMPD,
  discoverRenderers,
  verifyMPD,
} from '../audioplayer/jukebox'
import XiaomiAuthBlock from './XiaomiAuthBlock'
import { EMPTY_OUTPUT, SECRET_MASK, suggestId } from './outputConstants'

// The three supported output kinds, in the order they are offered. Each one
// owns its icon/colour so the cards, the list and the type chip all agree.
const TYPES = [
  {
    id: 'xiaomi',
    icon: MdSpeaker,
    color: '#ff6900',
    titleKey: 'resources.jukeboxOutput.typeCards.xiaomi.title',
    descKey: 'resources.jukeboxOutput.typeCards.xiaomi.desc',
  },
  {
    id: 'mpd',
    icon: MdCastConnected,
    color: '#3f51b5',
    titleKey: 'resources.jukeboxOutput.typeCards.mpd.title',
    descKey: 'resources.jukeboxOutput.typeCards.mpd.desc',
  },
  {
    id: 'dlna',
    icon: MdCastConnected,
    color: '#00897b',
    titleKey: 'resources.jukeboxOutput.typeCards.dlna.title',
    descKey: 'resources.jukeboxOutput.typeCards.dlna.desc',
  },
]

const useStyles = makeStyles((theme) => ({
  // Step header: three dots with labels, the active one highlighted.
  steps: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    padding: theme.spacing(1.5, 3),
    borderBottom: `1px solid ${theme.palette.divider}`,
    background:
      theme.palette.type === 'dark'
        ? 'rgba(255, 255, 255, 0.03)'
        : 'rgba(0, 0, 0, 0.02)',
  },
  step: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.75),
    fontSize: '0.82rem',
    color: theme.palette.text.secondary,
    background: 'none',
    border: 'none',
    padding: 0,
    cursor: 'pointer',
    fontFamily: 'inherit',
  },
  stepActive: { color: theme.palette.primary.main, fontWeight: 600 },
  stepDone: { color: theme.palette.text.primary },
  stepDot: {
    width: 22,
    height: 22,
    borderRadius: '50%',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: 12,
    fontWeight: 700,
    background: theme.palette.action.hover,
    color: theme.palette.text.secondary,
    flexShrink: 0,
  },
  stepDotActive: {
    backgroundColor: theme.palette.primary.main,
    color: theme.palette.primary.contrastText,
  },
  stepDotDone: {
    backgroundColor: theme.palette.success.main,
    color: '#fff',
  },
  stepLine: {
    flex: 1,
    height: 1,
    background: theme.palette.divider,
    minWidth: 12,
  },
  body: { padding: theme.spacing(2.5, 3, 1) },

  // Type cards: the whole point is that this choice is obvious at a glance.
  typeGrid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))',
    gap: theme.spacing(1.5),
  },
  typeCard: {
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(1),
    padding: theme.spacing(2),
    textAlign: 'left',
    cursor: 'pointer',
    borderRadius: theme.shape.borderRadius,
    border: `2px solid ${theme.palette.divider}`,
    background: theme.palette.background.paper,
    fontFamily: 'inherit',
    transition: theme.transitions.create('border-color', {
      duration: theme.transitions.duration.short,
    }),
    '&:hover': { borderColor: theme.palette.action.active },
  },
  typeCardActive: { borderColor: theme.palette.primary.main },
  typeCardTop: { display: 'flex', alignItems: 'center', gap: theme.spacing(1) },
  typeCardIcon: { fontSize: 22 },
  typeCardTitle: { fontWeight: 600, fontSize: '0.95rem' },
  typeCardDesc: {
    fontSize: '0.78rem',
    color: theme.palette.text.secondary,
    lineHeight: 1.5,
  },
  typeCardCheck: { marginLeft: 'auto', color: theme.palette.primary.main },

  picked: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    marginTop: theme.spacing(2),
    padding: theme.spacing(1.25),
    borderRadius: theme.shape.borderRadius,
    background: theme.palette.action.hover,
    fontSize: '0.85rem',
  },

  sectionTitle: {
    fontWeight: 600,
    fontSize: '0.95rem',
    color: theme.palette.text.primary,
    marginBottom: theme.spacing(1.5),
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
  },
  row: {
    display: 'flex',
    gap: theme.spacing(2),
    flexWrap: 'wrap',
    '& > *': { flex: '1 1 200px' },
  },
  hint: {
    fontSize: '0.78rem',
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(0.5),
    lineHeight: 1.6,
  },
  field: { marginTop: theme.spacing(1) },
  fieldTight: { marginTop: theme.spacing(0.5) },
  scanHead: {
    display: 'flex',
    gap: theme.spacing(1),
    alignItems: 'center',
    flexWrap: 'wrap',
  },
  results: {
    marginTop: theme.spacing(1),
    maxHeight: 160,
    overflowY: 'auto',
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
  },
  resultItem: {
    padding: theme.spacing(1),
    cursor: 'pointer',
    fontSize: '0.8rem',
    borderBottom: `1px solid ${theme.palette.divider}`,
    '&:last-child': { borderBottom: 'none' },
    '&:hover': { background: theme.palette.action.hover },
  },
  mpdItem: {
    padding: theme.spacing(1),
    cursor: 'pointer',
    fontSize: '0.8rem',
    borderBottom: `1px solid ${theme.palette.divider}`,
    display: 'flex',
    gap: theme.spacing(1),
    alignItems: 'center',
    '&:last-child': { borderBottom: 'none' },
    '&:hover': { background: theme.palette.action.hover },
  },
  mpdItemText: { flex: 1, minWidth: 0 },
  error: { color: theme.palette.error.main, fontSize: '0.8rem' },
  ok: { color: theme.palette.success.main, fontSize: '0.8rem' },
  footer: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    padding: theme.spacing(1.5, 3),
    borderTop: `1px solid ${theme.palette.divider}`,
  },
  grow: { flex: 1 },
}))

// react-final-form bound MUI TextField: it reads value/handlers from the form
// context itself, so callers only pass name + label.
const Input = ({
  input,
  label,
  type = 'text',
  helperText,
  disabled,
  ...rest
}) => {
  const translate = useTranslate()
  return (
    <TextField
      {...input}
      {...rest}
      type={type}
      label={translate(label, { _: label })}
      helperText={helperText}
      disabled={disabled}
      variant="outlined"
      size="small"
      fullWidth
      margin="dense"
    />
  )
}

Input.propTypes = {
  input: PropTypes.object.isRequired,
  label: PropTypes.string,
  type: PropTypes.string,
  helperText: PropTypes.node,
  disabled: PropTypes.bool,
}

// Network scan for the output being edited: SSDP multicast for DLNA, port
// probing for MPD. Both fill the address (and name) with whatever is found.
const ScanPanel = ({ type }) => {
  const classes = useStyles()
  const translate = useTranslate()
  const form = useForm()
  const [scanning, setScanning] = useState(false)
  const [results, setResults] = useState(null)
  const [error, setError] = useState(null)
  const isMPD = type === 'mpd'

  const scan = useCallback(() => {
    setScanning(true)
    setError(null)
    const done = (list) => {
      setResults(list)
      setScanning(false)
    }
    const fail = (e) => {
      setError(e.message || 'scan failed')
      setResults([])
      setScanning(false)
    }
    if (isMPD) {
      discoverMPD({ timeout: 4 }).then(done).catch(fail)
    } else {
      discoverRenderers(6).then(done).catch(fail)
    }
  }, [isMPD])

  const pick = (device) => {
    if (isMPD) {
      form.change('address', device.address)
      if (!form.values?.name) {
        form.change('name', `MPD ${device.host}`)
      }
      return
    }
    form.change('address', device.address)
    if (device.name) {
      form.change('name', device.name)
    }
  }

  return (
    <Box className={classes.field}>
      <div className={classes.scanHead}>
        <Button
          size="small"
          variant="outlined"
          startIcon={scanning ? <CircularProgress size={14} /> : <MdSearch />}
          onClick={scan}
          disabled={scanning}
        >
          {isMPD
            ? translate('resources.jukeboxOutput.actions.scanMpd', {
                _: '扫描局域网内的 MPD',
              })
            : translate('resources.jukeboxOutput.actions.scan', {
                _: '扫描局域网设备',
              })}
        </Button>
        {isMPD && (
          <Typography variant="caption" color="textSecondary">
            {translate('resources.jukeboxOutput.hint.mpdScanHint', {
              _: 'MPD 没有广播机制，扫描会依次探测本机各网段的 6600 端口（约 1-2 秒）。',
            })}
          </Typography>
        )}
        {results && !isMPD && (
          <Typography variant="caption" color="textSecondary">
            {results.length}{' '}
            {translate('resources.jukeboxOutput.scanFound', { _: '台设备' })}
          </Typography>
        )}
      </div>
      {error && <div className={classes.error}>{error}</div>}
      {results && results.length > 0 && (
        <div className={classes.results}>
          {results.map((device) => (
            <div
              key={device.address}
              className={isMPD ? classes.mpdItem : classes.resultItem}
              onClick={() => pick(device)}
            >
              {isMPD ? (
                <>
                  <div className={classes.mpdItemText}>
                    <strong>{device.address}</strong>
                    {device.version && (
                      <span style={{ opacity: 0.7 }}>
                        {' '}
                        · MPD {device.version}
                      </span>
                    )}
                  </div>
                  {device.needsPassword && (
                    <Chip
                      size="small"
                      label={translate(
                        'resources.jukeboxOutput.hint.needsPassword',
                        { _: '需要密码' },
                      )}
                      style={{ height: 20 }}
                    />
                  )}
                </>
              ) : (
                <>
                  <strong>{device.name || device.usn || 'renderer'}</strong>
                  <br />
                  <span style={{ opacity: 0.7 }}>{device.address}</span>
                </>
              )}
            </div>
          ))}
        </div>
      )}
      {results && results.length === 0 && (
        <Typography variant="caption" color="textSecondary">
          {translate('resources.jukeboxOutput.hint.scanEmpty', {
            _: '没找到设备。可以手动填写地址，或确认设备与本服务在同一网段。',
          })}
        </Typography>
      )}
    </Box>
  )
}

ScanPanel.propTypes = { type: PropTypes.string }

// Tests the address/password with a real MPD login before the output is saved.
const MpdVerify = ({ address, password }) => {
  const classes = useStyles()
  const translate = useTranslate()
  const [state, setState] = useState(null) // null | 'testing' | 'ok' | error string
  const dirty = useRef(false)

  useEffect(() => {
    dirty.current = true
  }, [address, password])

  const run = useCallback(() => {
    if (!address?.trim()) {
      return
    }
    dirty.current = false
    setState('testing')
    verifyMPD(address.trim(), password === SECRET_MASK ? '' : password)
      .then(() => setState('ok'))
      .catch((e) => {
        if (dirty.current) {
          return
        }
        setState(e.message || 'verify failed')
      })
  }, [address, password])

  return (
    <div className={classes.field}>
      <div className={classes.scanHead}>
        <Button
          size="small"
          variant="outlined"
          startIcon={
            state === 'testing' ? <CircularProgress size={14} /> : <MdWifi />
          }
          onClick={run}
          disabled={!address?.trim() || state === 'testing'}
        >
          {translate('resources.jukeboxOutput.actions.verifyMpd', {
            _: '测试连接',
          })}
        </Button>
        {state === 'ok' && (
          <span className={classes.ok}>
            {translate('resources.jukeboxOutput.messages.mpdOk', {
              _: '连接成功，密码正确。',
            })}
          </span>
        )}
        {state && state !== 'ok' && state !== 'testing' && (
          <span className={classes.error}>{state}</span>
        )}
      </div>
    </div>
  )
}

MpdVerify.propTypes = { address: PropTypes.string, password: PropTypes.string }

// The ID field is special: until the user types in it, it follows the name
// (most people never need to think about the ID at all).
const IdField = ({ helperText, disabled, onTouched }) => {
  const field = useField('id')
  const translate = useTranslate()
  return (
    <TextField
      {...field}
      // useField's return value does not always carry the name through to the
      // DOM, and the label/for pair needs it.
      name="id"
      onFocus={(e) => {
        field.onFocus(e)
        onTouched?.()
      }}
      label={translate('resources.jukeboxOutput.fields.id', { _: 'ID' })}
      helperText={helperText}
      disabled={disabled}
      variant="outlined"
      size="small"
      fullWidth
      margin="dense"
    />
  )
}

IdField.propTypes = {
  helperText: PropTypes.node,
  disabled: PropTypes.bool,
  onTouched: PropTypes.func,
}

// The type chooser: three big cards instead of a dropdown, so what the device
// is decides the rest of the form at a glance.
const TypePicker = ({ value, onChange }) => {
  const classes = useStyles()
  const translate = useTranslate()
  return (
    <Box>
      <div className={classes.typeGrid}>
        {TYPES.map((t) => {
          const Icon = t.icon
          const active = value === t.id
          return (
            <button
              type="button"
              key={t.id}
              className={`${classes.typeCard} ${
                active ? classes.typeCardActive : ''
              }`}
              aria-pressed={active}
              onClick={() => onChange(t.id)}
            >
              <div className={classes.typeCardTop}>
                <Icon
                  className={classes.typeCardIcon}
                  style={{ color: t.color }}
                />
                <span className={classes.typeCardTitle}>
                  {translate(t.titleKey, { _: t.id })}
                </span>
                {active && (
                  <MdCheck
                    className={classes.typeCardCheck}
                    titleAccess={translate('ra.action.confirm', {
                      _: '已选择',
                    })}
                  />
                )}
              </div>
              <div className={classes.typeCardDesc}>
                {translate(t.descKey, { _: '' })}
              </div>
            </button>
          )
        })}
      </div>
    </Box>
  )
}

TypePicker.propTypes = {
  value: PropTypes.string,
  onChange: PropTypes.func.isRequired,
}

const OutputEditorDialog = ({ mode, output, onClose, onSaved }) => {
  const classes = useStyles()
  const translate = useTranslate()
  const notify = useNotify()
  const isCreate = mode === 'create'
  // Editing an existing device: open on the connection step, the type is already
  // decided and shown as a chip.
  const [step, setStep] = useState(isCreate ? 0 : 1)
  // false until the user actually types in the ID field; editing an existing
  // device starts as "touched" so its stored ID is never rewritten.
  const idTouched = useRef(!isCreate)

  const initialValues = { ...EMPTY_OUTPUT, ...(output || {}) }

  const save = useCallback(
    async (values) => {
      const payload = { ...values }
      payload.type = (payload.type || 'xiaomi').toLowerCase()
      // The ID is an implementation detail for the config file and the API, so
      // derive it from the name instead of making the user invent one.
      if (!payload.id?.trim()) {
        payload.id = suggestId(values.name, payload.type)
      }
      try {
        if (isCreate) {
          await httpClient('/api/jukebox/outputs', {
            method: 'POST',
            headers: new Headers({ 'Content-Type': 'application/json' }),
            body: JSON.stringify(payload),
          })
        } else {
          await httpClient(
            `/api/jukebox/outputs/${encodeURIComponent(values.id)}`,
            {
              method: 'PUT',
              headers: new Headers({ 'Content-Type': 'application/json' }),
              body: JSON.stringify(payload),
            },
          )
        }
        notify('resources.jukeboxOutput.messages.saved', {
          type: 'info',
          messageOptions: { defaultMessage: '输出设备已保存' },
        })
        onSaved()
      } catch (e) {
        notify('ra.page.error', {
          type: 'warning',
          messageArgs: { error: e.message },
        })
      }
    },
    [isCreate, notify, onSaved],
  )

  const validate = useCallback(
    (values) => {
      const errors = {}
      const type = (values.type || 'xiaomi').toLowerCase()
      const effectiveId = values.id?.trim() || suggestId(values.name, type)
      if (!effectiveId) {
        errors.id = translate('resources.jukeboxOutput.errors.idRequired', {
          _: '需要填写 ID',
        })
      } else if (!/^[a-zA-Z0-9_-]{1,64}$/.test(effectiveId)) {
        errors.id = translate('resources.jukeboxOutput.errors.idInvalid', {
          _: 'ID 只能包含字母、数字、下划线和短横线（1-64 位）',
        })
      }
      if (!values.name?.trim()) {
        errors.name = translate('resources.jukeboxOutput.errors.nameRequired', {
          _: '需要填写名称',
        })
      }
      if (!values.address?.trim()) {
        errors.address = translate(
          'resources.jukeboxOutput.errors.addressRequired',
          {
            _: '需要填写地址',
          },
        )
      }
      return errors
    },
    [translate],
  )

  const STEPS = [
    {
      key: 'type',
      label: 'resources.jukeboxOutput.steps.type',
      fallback: '1. 类型',
    },
    {
      key: 'connection',
      label: 'resources.jukeboxOutput.steps.connection',
      fallback: '2. 连接',
    },
    {
      key: 'advanced',
      label: 'resources.jukeboxOutput.steps.advanced',
      fallback: '3. 高级',
    },
  ]

  return (
    <Dialog open onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle style={{ paddingBottom: 8 }}>
        {isCreate
          ? translate('resources.jukeboxOutput.createTitle', {
              _: '新增输出设备',
            })
          : translate('resources.jukeboxOutput.editTitle', {
              _: '编辑输出设备',
            })}
      </DialogTitle>
      <Form
        initialValues={initialValues}
        onSubmit={save}
        validate={validate}
        render={({ handleSubmit, form, submitting, values, errors }) => {
          const type = values.type || 'xiaomi'
          const meta = TYPES.find((t) => t.id === type) || TYPES[0]
          const Icon = meta.icon
          // Step 2 is the only one with required fields; the last step is optional.
          const connectionIncomplete =
            !values.name?.trim() || !values.address?.trim()
          // Preview of the ID that will be stored when the field is left empty.
          const autoId =
            isCreate && !idTouched.current && !values.id?.trim()
              ? suggestId(values.name, type)
              : ''
          // The ID requirement is satisfied by a typed ID or by the generated one.
          const idSatisfied = Boolean(values.id?.trim() || autoId)

          return (
            <form onSubmit={handleSubmit} style={{ display: 'contents' }}>
              <div className={classes.steps}>
                {STEPS.map((s, i) => {
                  const active = step === i
                  const done = i < step
                  return (
                    <React.Fragment key={s.key}>
                      {i > 0 && <span className={classes.stepLine} />}
                      <button
                        type="button"
                        className={`${classes.step} ${
                          active
                            ? classes.stepActive
                            : done
                              ? classes.stepDone
                              : ''
                        }`}
                        onClick={() => setStep(i)}
                      >
                        <span
                          className={`${classes.stepDot} ${
                            active
                              ? classes.stepDotActive
                              : done
                                ? classes.stepDotDone
                                : ''
                          }`}
                        >
                          {done ? <MdCheck fontSize="small" /> : i + 1}
                        </span>
                        {translate(s.label, { _: s.fallback })}
                      </button>
                    </React.Fragment>
                  )
                })}
              </div>

              <DialogContent className={classes.body}>
                {step === 0 && (
                  <TypePicker
                    value={type}
                    onChange={(next) => {
                      form.change('type', next)
                      setStep(1)
                    }}
                  />
                )}

                {step === 1 && (
                  <Box>
                    {!isCreate && (
                      <div className={classes.picked}>
                        <Icon style={{ color: meta.color }} />
                        <span style={{ flex: 1 }}>
                          {translate(meta.titleKey, { _: type })}
                        </span>
                        <Button
                          size="small"
                          onClick={() => setStep(0)}
                          style={{ minWidth: 0, padding: '0 8px' }}
                        >
                          {translate(
                            'resources.jukeboxOutput.actions.changeType',
                            {
                              _: '更换',
                            },
                          )}
                        </Button>
                      </div>
                    )}

                    <div
                      className={classes.sectionTitle}
                      style={{ marginTop: isCreate ? 0 : 16 }}
                    >
                      {type === 'xiaomi'
                        ? translate('resources.jukeboxOutput.sections.xiaomi', {
                            _: '小爱音箱',
                          })
                        : type === 'mpd'
                          ? translate('resources.jukeboxOutput.sections.mpd', {
                              _: 'MPD',
                            })
                          : translate('resources.jukeboxOutput.sections.dlna', {
                              _: 'DLNA / UPnP',
                            })}
                    </div>

                    {type === 'xiaomi' && (
                      <Box>
                        <div className={classes.row}>
                          <Field
                            name="name"
                            component={Input}
                            label="resources.jukeboxOutput.fields.name"
                            autoFocus={isCreate}
                          />
                          <IdField
                            disabled={!isCreate}
                            helperText={
                              errors.id ||
                              (autoId
                                ? translate(
                                    'resources.jukeboxOutput.helpers.idAuto',
                                    {
                                      _: '将自动生成为：{{id}}（可自行修改）',
                                      id: autoId,
                                    },
                                  )
                                : translate(
                                    'resources.jukeboxOutput.helpers.idHint',
                                    {
                                      _: '仅用于配置文件与接口调用，留空会按名称自动生成。',
                                    },
                                  ))
                            }
                            onTouched={() => {
                              idTouched.current = true
                            }}
                          />
                        </div>
                        <div className={classes.fieldTight}>
                          <XiaomiAuthBlock
                            formData={values}
                            isCreate={isCreate}
                            onBatchCreated={({ created, failed }) => {
                              // 已经建成输出设备了，刷新列表；全都成功就直接关掉弹窗，
                              // 有失败则留在页面上让用户看到 Alert
                              onSaved({ keepOpen: failed.length > 0 })
                              notify(
                                'resources.jukeboxOutput.messages.batchSaved',
                                {
                                  type: failed.length > 0 ? 'warning' : 'info',
                                  messageOptions: {
                                    defaultMessage:
                                      '已添加 {{count}} 台输出设备{{failed}}',
                                  },
                                  messageArgs: {
                                    count: created.length,
                                    failed: failed.length
                                      ? `，${failed.length} 台失败`
                                      : '',
                                  },
                                },
                              )
                            }}
                          />
                        </div>
                        <div className={classes.fieldTight}>
                          <Field
                            name="address"
                            component={Input}
                            label="resources.jukeboxOutput.fields.address"
                            placeholder="192.168.1.10"
                            helperText={translate(
                              'resources.jukeboxOutput.helpers.xiaomiAddress',
                              {
                                _: '音箱的局域网 IP，扫码后会自动填好。',
                              },
                            )}
                            error={Boolean(errors.address)}
                          />
                        </div>
                      </Box>
                    )}

                    {type === 'mpd' && (
                      <Box>
                        <div className={classes.row}>
                          <Field
                            name="name"
                            component={Input}
                            label="resources.jukeboxOutput.fields.name"
                            autoFocus={isCreate}
                          />
                          <IdField
                            disabled={!isCreate}
                            helperText={
                              errors.id ||
                              (autoId
                                ? translate(
                                    'resources.jukeboxOutput.helpers.idAuto',
                                    {
                                      _: '将自动生成为：{{id}}（可自行修改）',
                                      id: autoId,
                                    },
                                  )
                                : translate(
                                    'resources.jukeboxOutput.helpers.idHint',
                                    {
                                      _: '仅用于配置文件与接口调用，留空会按名称自动生成。',
                                    },
                                  ))
                            }
                            onTouched={() => {
                              idTouched.current = true
                            }}
                          />
                        </div>
                        <div className={classes.fieldTight}>
                          <ScanPanel type="mpd" />
                        </div>
                        <div className={classes.fieldTight}>
                          <Field
                            name="address"
                            component={Input}
                            label="resources.jukeboxOutput.fields.address"
                            placeholder="192.168.1.10:6600"
                            helperText={translate(
                              'resources.jukeboxOutput.helpers.mpdAddress',
                              {
                                _: '扫描最省事：多数 NAS 的 MPD 就在本机网段的 6600 端口。也可以手填 host:port。',
                              },
                            )}
                            error={Boolean(errors.address)}
                          />
                        </div>
                        <div className={classes.fieldTight}>
                          <Field
                            name="password"
                            component={Input}
                            type="password"
                            label="resources.jukeboxOutput.fields.mpdPassword"
                            helperText={
                              initialValues.password === SECRET_MASK
                                ? translate(
                                    'resources.jukeboxOutput.helpers.maskedSecret',
                                    { _: '已保存 —— 留空则保持原值不变' },
                                  )
                                : translate(
                                    'resources.jukeboxOutput.helpers.mpdPassword',
                                    {
                                      _: 'mpd.conf 里的 password 整行原样填入，例如 password "abc123@read,add,control"；留空表示允许匿名。',
                                    },
                                  )
                            }
                          />
                          <MpdVerify
                            address={values.address}
                            password={values.password}
                          />
                        </div>
                      </Box>
                    )}

                    {type === 'dlna' && (
                      <Box>
                        <div className={classes.row}>
                          <Field
                            name="name"
                            component={Input}
                            label="resources.jukeboxOutput.fields.name"
                            autoFocus={isCreate}
                          />
                          <IdField
                            disabled={!isCreate}
                            helperText={
                              errors.id ||
                              (autoId
                                ? translate(
                                    'resources.jukeboxOutput.helpers.idAuto',
                                    {
                                      _: '将自动生成为：{{id}}（可自行修改）',
                                      id: autoId,
                                    },
                                  )
                                : translate(
                                    'resources.jukeboxOutput.helpers.idHint',
                                    {
                                      _: '仅用于配置文件与接口调用，留空会按名称自动生成。',
                                    },
                                  ))
                            }
                            onTouched={() => {
                              idTouched.current = true
                            }}
                          />
                        </div>
                        <div className={classes.fieldTight}>
                          <ScanPanel type="dlna" />
                        </div>
                        <div className={classes.fieldTight}>
                          <Field
                            name="address"
                            component={Input}
                            label="resources.jukeboxOutput.fields.address"
                            placeholder="http://192.168.1.10:8200/xxxx.xml"
                            helperText={translate(
                              'resources.jukeboxOutput.helpers.dlnaAddress',
                              {
                                _: '扫描后点选结果即可；填设备描述文档 URL（不是 /AVTransport/control）。',
                              },
                            )}
                            error={Boolean(errors.address)}
                          />
                        </div>
                      </Box>
                    )}
                  </Box>
                )}

                {step === 2 && (
                  <Box>
                    <div
                      className={classes.hint}
                      style={{ marginTop: 0, marginBottom: 16 }}
                    >
                      {translate(
                        'resources.jukeboxOutput.helpers.advancedHint',
                        {
                          _: '一般不需要填。只有扫码/手动配置失败、需要手工指定令牌或路径映射时才用得上。',
                        },
                      )}
                    </div>
                    {type === 'xiaomi' ? (
                      <>
                        <div className={classes.row}>
                          <Field
                            name="token"
                            component={Input}
                            label="resources.jukeboxOutput.fields.token"
                            helperText={translate(
                              'resources.jukeboxOutput.helpers.token',
                              {
                                _: '32 位十六进制 miIO 令牌，用于局域网控制',
                              },
                            )}
                          />
                          <Field
                            name="did"
                            component={Input}
                            label="resources.jukeboxOutput.fields.did"
                          />
                        </div>
                        <div className={classes.row}>
                          <Field
                            name="model"
                            component={Input}
                            label="resources.jukeboxOutput.fields.model"
                            placeholder="l7a / s12 / l05b"
                          />
                          <Field
                            name="textDirective"
                            component={Input}
                            label="resources.jukeboxOutput.fields.textDirective"
                            placeholder="siid-aiid"
                          />
                        </div>
                        <div className={classes.row}>
                          <Field
                            name="account"
                            component={Input}
                            label="resources.jukeboxOutput.fields.account"
                          />
                          <Field
                            name="password"
                            component={Input}
                            type="password"
                            label="resources.jukeboxOutput.fields.accountPassword"
                            helperText={
                              initialValues.password === SECRET_MASK
                                ? translate(
                                    'resources.jukeboxOutput.helpers.maskedSecret',
                                    { _: '已保存 —— 留空则保持原值不变' },
                                  )
                                : undefined
                            }
                          />
                        </div>
                        <Field
                          name="passToken"
                          component={Input}
                          type="password"
                          label="resources.jukeboxOutput.fields.passToken"
                        />
                      </>
                    ) : (
                      <>
                        {type === 'mpd' && (
                          <>
                            <div className={classes.sectionTitle}>
                              {translate(
                                'resources.jukeboxOutput.sections.pathMap',
                                {
                                  _: '路径映射（可选）',
                                },
                              )}
                            </div>
                            <div className={classes.row}>
                              <Field
                                name="pathFrom"
                                component={Input}
                                label="resources.jukeboxOutput.fields.pathFrom"
                              />
                              <Field
                                name="pathTo"
                                component={Input}
                                label="resources.jukeboxOutput.fields.pathTo"
                              />
                            </div>
                            <div className={classes.hint}>
                              {translate(
                                'resources.jukeboxOutput.helpers.pathHint',
                                {
                                  _: '仅在 MPD 曲库目录与本服务 MusicFolder 不一致时使用：把歌曲路径中的一段替换成另一段。',
                                },
                              )}
                            </div>
                          </>
                        )}
                        {type === 'dlna' && (
                          <div className={classes.hint}>
                            {translate(
                              'resources.jukeboxOutput.helpers.dlnaNoExtra',
                              {
                                _: 'DLNA 不需要额外设置，保存即可使用。',
                              },
                            )}
                          </div>
                        )}
                      </>
                    )}
                  </Box>
                )}
              </DialogContent>

              <div className={classes.footer}>
                {step > 0 && (
                  <Button
                    onClick={() => setStep(step - 1)}
                    disabled={submitting}
                  >
                    {translate('resources.jukeboxOutput.actions.prev', {
                      _: '上一步',
                    })}
                  </Button>
                )}
                <div className={classes.grow} />
                {step < STEPS.length - 1 ? (
                  <Button
                    variant="contained"
                    color="primary"
                    disableElevation
                    onClick={() => setStep(step + 1)}
                    disabled={step === 1 && connectionIncomplete}
                  >
                    {translate('resources.jukeboxOutput.actions.next', {
                      _: '下一步',
                    })}
                  </Button>
                ) : (
                  <Button
                    type="submit"
                    variant="contained"
                    color="primary"
                    disableElevation
                    disabled={
                      submitting || connectionIncomplete || !idSatisfied
                    }
                  >
                    {submitting ? (
                      <CircularProgress size={18} />
                    ) : (
                      translate('ra.action.save', { _: '保存' })
                    )}
                  </Button>
                )}
              </div>
            </form>
          )
        }}
      />
    </Dialog>
  )
}

OutputEditorDialog.propTypes = {
  mode: PropTypes.oneOf(['create', 'edit']).isRequired,
  output: PropTypes.object,
  onClose: PropTypes.func.isRequired,
  onSaved: PropTypes.func.isRequired,
}

export default OutputEditorDialog
