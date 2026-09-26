import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { makeStyles } from '@material-ui/core/styles'
import Button from '@material-ui/core/Button'
import Card from '@material-ui/core/Card'
import CardContent from '@material-ui/core/CardContent'
import Chip from '@material-ui/core/Chip'
import CircularProgress from '@material-ui/core/CircularProgress'
import Divider from '@material-ui/core/Divider'
import Link from '@material-ui/core/Link'
import List from '@material-ui/core/List'
import ListItem from '@material-ui/core/ListItem'
import ListItemSecondaryAction from '@material-ui/core/ListItemSecondaryAction'
import ListItemText from '@material-ui/core/ListItemText'
import Alert from '@material-ui/lab/Alert'
import Checkbox from '@material-ui/core/Checkbox'
import Tab from '@material-ui/core/Tab'
import Tabs from '@material-ui/core/Tabs'
import TextField from '@material-ui/core/TextField'
import Typography from '@material-ui/core/Typography'
import CheckCircleIcon from '@material-ui/icons/CheckCircle'
import CropFreeIcon from '@material-ui/icons/CropFree'
import ExpandLessIcon from '@material-ui/icons/ExpandLess'
import ExpandMoreIcon from '@material-ui/icons/ExpandMore'
import HelpOutlineIcon from '@material-ui/icons/HelpOutline'
import LockOpenIcon from '@material-ui/icons/LockOpen'
import RefreshIcon from '@material-ui/icons/Refresh'
import PlaylistAddIcon from '@material-ui/icons/PlaylistAdd'
import SpeakerIcon from '@material-ui/icons/Speaker'
import VpnKeyIcon from '@material-ui/icons/VpnKey'
import { useForm } from 'react-final-form'
import {
  initXiaomiQR,
  pollXiaomiQR,
  loginXiaomiPassword,
  loginXiaomiPassToken,
} from '../audioplayer/jukebox'
import { uniqueOutputId } from './outputConstants'

const useStyles = makeStyles((theme) => ({
  batchBar: {
    display: 'flex',
    alignItems: 'center',
    flexWrap: 'wrap',
    gap: theme.spacing(1),
    marginTop: theme.spacing(1),
    marginBottom: theme.spacing(0.5),
  },
  grow: {
    flex: '1 1 auto',
  },
  root: {
    marginTop: theme.spacing(1.5),
    marginBottom: theme.spacing(2),
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
    background:
      theme.palette.type === 'dark'
        ? 'rgba(255, 255, 255, 0.03)'
        : 'rgba(0, 0, 0, 0.01)',
  },
  header: {
    padding: theme.spacing(1.5, 2),
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottom: `1px solid ${theme.palette.divider}`,
  },
  titleWrap: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
  },
  tabs: {
    borderBottom: `1px solid ${theme.palette.divider}`,
    minHeight: 42,
  },
  tab: {
    minHeight: 42,
    fontSize: '0.85rem',
    fontWeight: 500,
    textTransform: 'none',
  },
  body: {
    padding: theme.spacing(2),
  },
  qrContainer: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: theme.spacing(1.5),
    padding: theme.spacing(1),
  },
  qrImage: {
    width: 200,
    height: 200,
    borderRadius: 8,
    border: `1px solid ${theme.palette.divider}`,
    boxShadow: theme.shadows[2],
    background: '#fff',
  },
  qrCountdown: {
    fontWeight: 600,
  },
  formRow: {
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(1.5),
    maxWidth: 420,
  },
  infoBanner: {
    padding: theme.spacing(1.2, 1.5),
    marginBottom: theme.spacing(1.5),
    borderRadius: 6,
    background:
      theme.palette.type === 'dark'
        ? 'rgba(33, 150, 243, 0.15)'
        : 'rgba(33, 150, 243, 0.08)',
    borderLeft: `4px solid ${theme.palette.info.main}`,
    fontSize: '0.85rem',
    lineHeight: 1.5,
  },
  errorBanner: {
    padding: theme.spacing(1, 1.5),
    marginTop: theme.spacing(1),
    borderRadius: 6,
    color: theme.palette.error.main,
    background:
      theme.palette.type === 'dark'
        ? 'rgba(244, 67, 54, 0.15)'
        : 'rgba(244, 67, 54, 0.08)',
    borderLeft: `4px solid ${theme.palette.error.main}`,
    fontSize: '0.85rem',
  },
  successCard: {
    padding: theme.spacing(1.5),
    background:
      theme.palette.type === 'dark'
        ? 'rgba(76, 175, 80, 0.12)'
        : 'rgba(76, 175, 80, 0.08)',
    border: `1px solid ${theme.palette.success.main}`,
    borderRadius: 6,
    marginBottom: theme.spacing(1.5),
  },
  deviceItem: {
    borderRadius: 6,
    border: `1px solid ${theme.palette.divider}`,
    marginBottom: theme.spacing(1),
    transition: 'all 0.2s ease',
    '&:hover': {
      background: theme.palette.action.hover,
      borderColor: theme.palette.primary.main,
    },
  },
  speakerItem: {
    background:
      theme.palette.type === 'dark'
        ? 'rgba(255, 152, 0, 0.08)'
        : 'rgba(255, 152, 0, 0.04)',
    borderColor:
      theme.palette.type === 'dark'
        ? 'rgba(255, 152, 0, 0.3)'
        : 'rgba(255, 152, 0, 0.4)',
  },
  deviceMeta: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: theme.spacing(0.75),
    marginTop: theme.spacing(0.5),
  },
  chip: {
    height: 22,
    fontSize: '0.75rem',
  },
}))

const isXiaomiSpeaker = (model = '') => {
  const m = model.toLowerCase()
  return (
    m.includes('speaker') ||
    m.includes('wifispeaker') ||
    m.includes('l7a') ||
    m.includes('l07a') ||
    m.includes('l05b') ||
    m.includes('l05c') ||
    m.includes('s12') ||
    m.includes('sound') ||
    m.includes('x08c') ||
    m.includes('lx06') ||
    m.includes('play')
  )
}

// 没有局域网 IP 的设备没法当输出用（驱动靠 IP 控制/拉流）
const isUsableDevice = (dev) => Boolean(dev?.localip)

const usableDids = (list) =>
  (list || []).filter(isUsableDevice).map((d) => d.did)

const XiaomiAuthBlock = ({ formData, isCreate, onBatchCreated }) => {
  const classes = useStyles()
  const form = useForm()

  const [tab, setTab] = useState(0) // 0: 扫码, 1: 密码, 2: passToken
  const [authResult, setAuthResult] = useState(null) // { userId, passToken, devices: [] }
  const [appliedDevice, setAppliedDevice] = useState(null)
  const [showOtherDevices, setShowOtherDevices] = useState(false)
  // 批量添加：勾选的设备 DID、已创建成功的 DID、创建过程与结果
  const [selectedDids, setSelectedDids] = useState([])
  const [createdDids, setCreatedDids] = useState([])
  const [batchBusy, setBatchBusy] = useState(false)
  const [batchResult, setBatchResult] = useState(null)

  // QR state
  const [qrInfo, setQrInfo] = useState(null)
  const [qrLoading, setQrLoading] = useState(false)
  const [qrError, setQrError] = useState(null)
  const [qrCountdown, setQrCountdown] = useState(0)
  const isPollingRef = useRef(false)
  const pollCancelRef = useRef(false)

  // Password state
  const [pwdAccount, setPwdAccount] = useState(formData.account || '')
  const [pwdPassword, setPwdPassword] = useState('')
  const [pwdLoading, setPwdLoading] = useState(false)
  const [pwdError, setPwdError] = useState(null)

  // PassToken state
  const [ptUserId, setPtUserId] = useState(formData.account || '')
  const [ptToken, setPtToken] = useState(formData.passToken || '')
  const [ptLoading, setPtLoading] = useState(false)
  const [ptError, setPtError] = useState(null)

  // Clean polling on unmount
  useEffect(() => {
    return () => {
      pollCancelRef.current = true
    }
  }, [])

  // QR Countdown timer
  useEffect(() => {
    if (qrCountdown <= 0) return
    const timer = setInterval(() => {
      setQrCountdown((prev) => Math.max(0, prev - 1))
    }, 1000)
    return () => clearInterval(timer)
  }, [qrCountdown])

  // Start QR login
  const startQR = useCallback(() => {
    setQrLoading(true)
    setQrError(null)
    pollCancelRef.current = false

    initXiaomiQR()
      .then((info) => {
        setQrInfo(info)
        setQrCountdown(info.timeout || 300)
        setQrLoading(false)

        // Start long polling
        if (!isPollingRef.current && info.lp) {
          isPollingRef.current = true
          const doPoll = (lpUrl) => {
            if (pollCancelRef.current) {
              isPollingRef.current = false
              return
            }
            pollXiaomiQR(lpUrl)
              .then((res) => {
                if (pollCancelRef.current) {
                  isPollingRef.current = false
                  return
                }
                if (res.status === 'success') {
                  isPollingRef.current = false
                  setAuthResult(res)
                } else if (res.status === 'waiting') {
                  // Keep polling
                  setTimeout(() => doPoll(lpUrl), 1000)
                }
              })
              .catch((err) => {
                if (pollCancelRef.current) return
                isPollingRef.current = false
                setQrError(err.message || '扫码轮询失败')
              })
          }
          doPoll(info.lp)
        }
      })
      .catch((err) => {
        setQrLoading(false)
        setQrError(err.message || '获取登录二维码失败')
      })
  }, [])

  // Password login handler
  const handlePasswordLogin = useCallback(() => {
    if (!pwdAccount || !pwdPassword) {
      setPwdError('请输入小米账号和密码')
      return
    }
    setPwdLoading(true)
    setPwdError(null)
    loginXiaomiPassword(pwdAccount, pwdPassword)
      .then((res) => {
        setPwdLoading(false)
        setAuthResult(res)
      })
      .catch((err) => {
        setPwdLoading(false)
        setPwdError(
          err.message || '账号密码登录失败，若触发验证码请改用扫码授权',
        )
      })
  }, [pwdAccount, pwdPassword])

  // PassToken login handler
  const handlePassTokenLogin = useCallback(() => {
    if (!ptUserId || !ptToken) {
      setPtError('请输入 userId 与 passToken')
      return
    }
    setPtLoading(true)
    setPtError(null)
    loginXiaomiPassToken(ptUserId, ptToken)
      .then((res) => {
        setPtLoading(false)
        setAuthResult(res)
      })
      .catch((err) => {
        setPtLoading(false)
        setPtError(err.message || 'passToken 凭证校验失败，请检查凭证是否有效')
      })
  }, [ptUserId, ptToken])

  const [refreshLoading, setRefreshLoading] = useState(false)
  const handleRefreshDevices = useCallback(() => {
    if (!authResult?.userId || !authResult?.passToken) return
    setRefreshLoading(true)
    loginXiaomiPassToken(authResult.userId, authResult.passToken)
      .then((res) => {
        setRefreshLoading(false)
        setAuthResult(res)
      })
      .catch((err) => {
        setRefreshLoading(false)
        alert('刷新设备失败: ' + err.message)
      })
  }, [authResult])

  // Device pick handler
  const handlePickDevice = useCallback(
    (dev) => () => {
      if (dev.localip) form.change('address', dev.localip)
      if (dev.name) form.change('name', dev.name)
      if (dev.token) form.change('token', dev.token)
      if (dev.did) form.change('did', dev.did)
      if (dev.model) {
        const cleanModel = dev.model.replace(/^xiaomi\.wifispeaker\./, '')
        form.change('model', cleanModel)
      }
      if (authResult?.userId) form.change('account', authResult.userId)
      if (authResult?.passToken) form.change('passToken', authResult.passToken)

      if (isCreate && (!formData.id || formData.id.startsWith('xiaomi_'))) {
        const suffix = dev.model
          ? dev.model
              .replace(/^xiaomi\.wifispeaker\./, '')
              .replace(/[^a-zA-Z0-9_-]/g, '')
          : dev.did || 'speaker'
        form.change('id', `xiaomi_${suffix}`)
      }

      setAppliedDevice(dev.did)
    },
    [authResult, form, formData.id, isCreate],
  )

  const devices = useMemo(() => authResult?.devices || [], [authResult])
  const speakerDevices = useMemo(
    () => devices.filter((d) => isXiaomiSpeaker(d.model)),
    [devices],
  )
  const otherDevices = useMemo(
    () => devices.filter((d) => !isXiaomiSpeaker(d.model)),
    [devices],
  )

  const toggleDevice = useCallback((did) => {
    setSelectedDids((prev) =>
      prev.includes(did) ? prev.filter((d) => d !== did) : [...prev, did],
    )
  }, [])

  const isCreated = useCallback(
    (did) => createdDids.includes(did),
    [createdDids],
  )

  const toggleSelectAll = useCallback(() => {
    setSelectedDids((prev) => {
      const all = usableDids(speakerDevices)
      return prev.length === all.length ? [] : all
    })
  }, [speakerDevices])

  // 一次登录常常能检索到十几台设备，逐台"选用此音箱"要点几十次。这里允许勾选多台，
  // 一次性建成多个输出设备——每台音箱都是一条独立输出，各自有 token/did/IP。
  const handleBatchCreate = useCallback(async () => {
    const targets = devices.filter(
      (d) => selectedDids.includes(d.did) && isUsableDevice(d),
    )
    if (targets.length === 0) return

    setBatchBusy(true)
    setBatchResult(null)
    const created = []
    const failed = []
    try {
      // 服务端不接受重复 ID，先把已存在的收进来，再逐台去重
      const taken = new Set()
      try {
        const res = await fetch('/api/jukebox/outputs')
        if (res.ok) {
          const list = await res.json()
          list.forEach((o) => taken.add(o.id))
        }
      } catch {
        // 读不到就靠后面的 400 兜底
      }

      for (const dev of targets) {
        const id = uniqueOutputId(dev.name, 'xiaomi', taken)
        taken.add(id)
        const payload = {
          id,
          name: dev.name || id,
          type: 'xiaomi',
          address: dev.localip,
        }
        if (dev.token) payload.token = dev.token
        if (dev.did) payload.did = dev.did
        if (dev.model) {
          payload.model = dev.model.replace(/^xiaomi\.wifispeaker\./, '')
        }
        if (authResult?.userId) payload.account = String(authResult.userId)
        if (authResult?.passToken) payload.passToken = authResult.passToken
        try {
          const res = await fetch('/api/jukebox/outputs', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
          })
          if (res.ok) {
            created.push(dev)
          } else {
            failed.push({ dev, reason: (await res.text()).trim() })
          }
        } catch (e) {
          failed.push({ dev, reason: e.message })
        }
      }
    } finally {
      setBatchBusy(false)
    }

    setCreatedDids((prev) => [...prev, ...created.map((d) => d.did)])
    setSelectedDids((prev) =>
      prev.filter((did) => !created.some((d) => d.did === did)),
    )
    setBatchResult({ created: created.length, failed })
    if (onBatchCreated) onBatchCreated({ created, failed })
  }, [devices, selectedDids, authResult, onBatchCreated])

  return (
    <Card className={classes.root} variant="outlined">
      <div className={classes.header}>
        <div className={classes.titleWrap}>
          <SpeakerIcon color="primary" />
          <div>
            <Typography variant="subtitle2" style={{ fontWeight: 600 }}>
              小爱音箱智能授权与自动发现
            </Typography>
            <Typography variant="caption" color="textSecondary">
              无需繁琐抓包，一键读取账号下的小爱音箱 IP、Token 与 DID
            </Typography>
          </div>
        </div>
        {authResult && (
          <Button
            size="small"
            color="secondary"
            onClick={() => {
              setAuthResult(null)
              setAppliedDevice(null)
              setQrInfo(null)
            }}
          >
            切换账号
          </Button>
        )}
      </div>

      {!authResult ? (
        <>
          <Tabs
            value={tab}
            onChange={(_, val) => setTab(val)}
            className={classes.tabs}
            indicatorColor="primary"
            textColor="primary"
            // 三个 tab 在手机上放不下，固定宽度会溢出屏幕，按 MUI 官方做法改成可滑动
            variant="scrollable"
            scrollButtons="auto"
          >
            <Tab
              icon={<CropFreeIcon style={{ fontSize: 18 }} />}
              label="米家扫码登录 (推荐)"
              className={classes.tab}
            />
            <Tab
              icon={<LockOpenIcon style={{ fontSize: 18 }} />}
              label="账号密码登录"
              className={classes.tab}
            />
            <Tab
              icon={<VpnKeyIcon style={{ fontSize: 18 }} />}
              label="passToken 凭证直填"
              className={classes.tab}
            />
          </Tabs>

          <div className={classes.body}>
            {/* 方案 1: 米家扫码登录 */}
            {tab === 0 && (
              <div className={classes.qrContainer}>
                {!qrInfo ? (
                  <>
                    <Typography variant="body2" color="textSecondary">
                      使用米家 App
                      或小米手机直接扫码，免输密码、安全快捷，自动规避风控验证。
                    </Typography>
                    <Button
                      variant="contained"
                      color="primary"
                      onClick={startQR}
                      disabled={qrLoading}
                      startIcon={
                        qrLoading ? (
                          <CircularProgress size={18} color="inherit" />
                        ) : (
                          <CropFreeIcon />
                        )
                      }
                    >
                      {qrLoading ? '正在生成二维码...' : '生成登录二维码'}
                    </Button>
                  </>
                ) : (
                  <>
                    <img
                      src={qrInfo.qr}
                      alt="Xiaomi Login QR"
                      className={classes.qrImage}
                    />
                    <div
                      style={{ display: 'flex', alignItems: 'center', gap: 8 }}
                    >
                      <CircularProgress size={16} />
                      <Typography variant="body2" style={{ fontWeight: 500 }}>
                        请使用米家 App 或小米手机扫码确认授权...
                      </Typography>
                    </div>
                    <Typography variant="caption" color="textSecondary">
                      打开手机【米家 App → 首页右上角扫一扫】或【设置 → 小米账号
                      → 右上角扫码】
                    </Typography>
                    <div
                      style={{ display: 'flex', alignItems: 'center', gap: 12 }}
                    >
                      <Chip
                        size="small"
                        label={`二维码有效期：${qrCountdown} 秒`}
                        color={qrCountdown > 30 ? 'default' : 'secondary'}
                        className={classes.qrCountdown}
                      />
                      <Button
                        size="small"
                        startIcon={<RefreshIcon />}
                        onClick={startQR}
                        disabled={qrLoading}
                      >
                        刷新
                      </Button>
                    </div>
                  </>
                )}
                {qrError && (
                  <div className={classes.errorBanner}>{qrError}</div>
                )}
              </div>
            )}

            {/* 方案 2: 账号密码登录 */}
            {tab === 1 && (
              <div className={classes.formRow}>
                <div className={classes.infoBanner}>
                  输入您的小米账号与密码。如遇到两步安全验证码，推荐使用左侧「米家扫码登录」或「passToken
                  凭证直填」直接免密授权。
                </div>
                <TextField
                  label="小米账号 (手机号 / 邮箱 / 小米 ID)"
                  variant="outlined"
                  size="small"
                  value={pwdAccount}
                  onChange={(e) => setPwdAccount(e.target.value)}
                  fullWidth
                />
                <TextField
                  label="账号密码"
                  type="password"
                  variant="outlined"
                  size="small"
                  value={pwdPassword}
                  onChange={(e) => setPwdPassword(e.target.value)}
                  fullWidth
                />
                <Button
                  variant="contained"
                  color="primary"
                  onClick={handlePasswordLogin}
                  disabled={pwdLoading || !pwdAccount || !pwdPassword}
                  startIcon={
                    pwdLoading && <CircularProgress size={18} color="inherit" />
                  }
                >
                  {pwdLoading ? '正在登录小米云...' : '登录并读取设备列表'}
                </Button>
                {pwdError && (
                  <div className={classes.errorBanner}>{pwdError}</div>
                )}
              </div>
            )}

            {/* 方案 3: passToken 直填 (配合 MI-PassToken-Helper) */}
            {tab === 2 && (
              <div className={classes.formRow}>
                <div className={classes.infoBanner}>
                  <div
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 4,
                      marginBottom: 4,
                    }}
                  >
                    <HelpOutlineIcon fontSize="small" />
                    <strong>备用方案：配合浏览器扩展免密获取</strong>
                  </div>
                  您可安装开源扩展{' '}
                  <Link
                    href="https://github.com/leriocn/MI-PassToken-Helper"
                    target="_blank"
                    rel="noreferrer"
                    style={{ fontWeight: 600, textDecoration: 'underline' }}
                  >
                    MI-PassToken-Helper
                  </Link>
                  ，在浏览器已登录的 <code>account.xiaomi.com</code> 一键提取{' '}
                  <code>userId</code> 与 <code>passToken</code>{' '}
                  凭证粘贴至此处。无需存储主密码、永不触发验证码且长期有效。
                </div>
                <TextField
                  label="小米账号 ID (userId，例如 1250258297)"
                  variant="outlined"
                  size="small"
                  value={ptUserId}
                  onChange={(e) => setPtUserId(e.target.value)}
                  fullWidth
                />
                <TextField
                  label="passToken 凭证 (例如 V1:xxxx...)"
                  variant="outlined"
                  size="small"
                  value={ptToken}
                  onChange={(e) => setPtToken(e.target.value)}
                  fullWidth
                />
                <Button
                  variant="contained"
                  color="primary"
                  onClick={handlePassTokenLogin}
                  disabled={ptLoading || !ptUserId || !ptToken}
                  startIcon={
                    ptLoading && <CircularProgress size={18} color="inherit" />
                  }
                >
                  {ptLoading ? '正在校验凭证...' : '校验凭证并读取设备列表'}
                </Button>
                {ptError && (
                  <div className={classes.errorBanner}>{ptError}</div>
                )}
              </div>
            )}
          </div>
        </>
      ) : (
        /* 授权成功态：展示设备列表供一键填入 */
        <div className={classes.body}>
          <div className={classes.successCard}>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                flexWrap: 'wrap',
                gap: 8,
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <CheckCircleIcon style={{ color: '#4caf50' }} />
                <div>
                  <Typography variant="subtitle2" style={{ fontWeight: 600 }}>
                    小米账号授权成功 (ID: {authResult.userId})
                  </Typography>
                  <Typography variant="caption" color="textSecondary">
                    共检索到 {devices.length}{' '}
                    台智能设备。勾选多台可一次性添加为输出设备；
                    单台则用「选用此音箱」填入下方表单。
                  </Typography>
                </div>
              </div>
              <Button
                size="small"
                variant="outlined"
                color="primary"
                onClick={handleRefreshDevices}
                disabled={refreshLoading}
                startIcon={
                  refreshLoading ? (
                    <CircularProgress size={14} color="inherit" />
                  ) : (
                    <RefreshIcon fontSize="small" />
                  )
                }
              >
                {refreshLoading ? '正在刷新...' : '刷新设备'}
              </Button>
            </div>
          </div>

          {speakerDevices.length > 0 && (
            <>
              <Typography
                variant="caption"
                style={{ fontWeight: 600, color: '#f57c00' }}
              >
                ⭐ 推荐小爱音箱设备 ({speakerDevices.length})
              </Typography>
              {speakerDevices.length > 0 && (
                <div className={classes.batchBar}>
                  <Button
                    size="small"
                    onClick={toggleSelectAll}
                    disabled={batchBusy}
                  >
                    {selectedDids.length === usableDids(speakerDevices).length
                      ? '取消全选'
                      : '全选'}
                  </Button>
                  <Typography variant="caption" color="textSecondary">
                    已选 {selectedDids.length} 台
                  </Typography>
                  <div className={classes.grow} />
                  <Button
                    size="small"
                    variant="contained"
                    color="primary"
                    onClick={handleBatchCreate}
                    disabled={batchBusy || selectedDids.length === 0}
                    startIcon={
                      batchBusy ? (
                        <CircularProgress size={14} color="inherit" />
                      ) : (
                        <PlaylistAddIcon fontSize="small" />
                      )
                    }
                  >
                    {batchBusy
                      ? '正在添加...'
                      : `添加所选 ${selectedDids.length} 台为输出设备`}
                  </Button>
                </div>
              )}
              {batchResult && (
                <Alert
                  severity={batchResult.failed.length ? 'warning' : 'success'}
                >
                  已添加 {batchResult.created} 台输出设备
                  {batchResult.failed.length > 0 &&
                    `；${batchResult.failed.length} 台失败：${batchResult.failed
                      .map((f) => `${f.dev.name}(${f.reason || '未知错误'})`)
                      .join('、')}`}
                </Alert>
              )}
              <List dense>
                {speakerDevices.map((dev) => (
                  <ListItem
                    key={dev.did}
                    className={`${classes.deviceItem} ${classes.speakerItem}`}
                  >
                    <Checkbox
                      edge="start"
                      size="small"
                      checked={selectedDids.includes(dev.did)}
                      disabled={!isUsableDevice(dev) || batchBusy}
                      onChange={() => toggleDevice(dev.did)}
                      inputProps={{ 'aria-label': `选择 ${dev.name}` }}
                    />
                    <ListItemText
                      secondaryTypographyProps={{ component: 'div' }}
                      primary={
                        <div
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 8,
                          }}
                        >
                          <SpeakerIcon
                            style={{ color: '#f57c00', fontSize: 20 }}
                          />
                          <span style={{ fontWeight: 600 }}>{dev.name}</span>
                          <Chip
                            size="small"
                            label="小爱音箱"
                            color="secondary"
                            className={classes.chip}
                          />
                          {dev.isOnline ? (
                            <Chip
                              size="small"
                              label="在线"
                              style={{
                                height: 20,
                                fontSize: '0.7rem',
                                background: '#4caf50',
                                color: '#fff',
                              }}
                            />
                          ) : (
                            <Chip
                              size="small"
                              label="离线"
                              style={{ height: 20, fontSize: '0.7rem' }}
                            />
                          )}
                        </div>
                      }
                      secondary={
                        <div className={classes.deviceMeta}>
                          <Chip
                            size="small"
                            label={`型号: ${dev.model}`}
                            className={classes.chip}
                          />
                          <Chip
                            size="small"
                            label={`局域网 IP: ${dev.localip || '未获取'}`}
                            color={dev.localip ? 'primary' : 'default'}
                            variant="outlined"
                            className={classes.chip}
                          />
                          <Chip
                            size="small"
                            label={`DID: ${dev.did}`}
                            className={classes.chip}
                          />
                          <Chip
                            size="small"
                            label={
                              dev.token ? '本地 Token: 已提取' : '无本地 Token'
                            }
                            className={classes.chip}
                          />
                        </div>
                      }
                    />
                    <ListItemSecondaryAction>
                      <Button
                        variant={
                          appliedDevice === dev.did || isCreated(dev.did)
                            ? 'outlined'
                            : 'contained'
                        }
                        color={
                          appliedDevice === dev.did || isCreated(dev.did)
                            ? 'default'
                            : 'primary'
                        }
                        size="small"
                        onClick={handlePickDevice(dev)}
                        disabled={!isUsableDevice(dev)}
                        startIcon={
                          (appliedDevice === dev.did || isCreated(dev.did)) && (
                            <CheckCircleIcon style={{ color: '#4caf50' }} />
                          )
                        }
                      >
                        {isCreated(dev.did)
                          ? '已添加'
                          : appliedDevice === dev.did
                            ? '已填入'
                            : '选用此音箱'}
                      </Button>
                    </ListItemSecondaryAction>
                  </ListItem>
                ))}
              </List>
            </>
          )}

          {otherDevices.length > 0 && (
            <div style={{ marginTop: 12 }}>
              <Divider style={{ marginBottom: 8 }} />
              <Button
                size="small"
                onClick={() => setShowOtherDevices((prev) => !prev)}
                style={{
                  textTransform: 'none',
                  color: 'inherit',
                  opacity: 0.75,
                  padding: '4px 8px',
                }}
                endIcon={
                  showOtherDevices ? (
                    <ExpandLessIcon fontSize="small" />
                  ) : (
                    <ExpandMoreIcon fontSize="small" />
                  )
                }
              >
                其他智能设备 ({otherDevices.length}){' '}
                {showOtherDevices ? '点击收起' : '点击展开查看'}
              </Button>
              {showOtherDevices && (
                <List dense style={{ maxHeight: 320, overflowY: 'auto' }}>
                  {otherDevices.map((dev) => (
                    <ListItem key={dev.did} className={classes.deviceItem}>
                      <ListItemText
                        secondaryTypographyProps={{ component: 'div' }}
                        primary={
                          <div
                            style={{
                              display: 'flex',
                              alignItems: 'center',
                              gap: 8,
                            }}
                          >
                            <span style={{ fontWeight: 500 }}>{dev.name}</span>
                            <Chip
                              size="small"
                              label={dev.model}
                              className={classes.chip}
                            />
                          </div>
                        }
                        secondary={
                          <div className={classes.deviceMeta}>
                            <Chip
                              size="small"
                              label={`IP: ${dev.localip || '无'}`}
                              className={classes.chip}
                            />
                            <Chip
                              size="small"
                              label={`DID: ${dev.did}`}
                              className={classes.chip}
                            />
                          </div>
                        }
                      />
                      <ListItemSecondaryAction>
                        <Button
                          variant="outlined"
                          size="small"
                          onClick={handlePickDevice(dev)}
                        >
                          {appliedDevice === dev.did ? '已填入' : '填入'}
                        </Button>
                      </ListItemSecondaryAction>
                    </ListItem>
                  ))}
                </List>
              )}
            </div>
          )}
        </div>
      )}
    </Card>
  )
}

export default XiaomiAuthBlock
