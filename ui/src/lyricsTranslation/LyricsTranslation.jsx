import React, { useEffect, useState, useCallback } from 'react'
import { Title, useNotify, useTranslate } from 'react-admin'
import {
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Collapse,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Divider,
  FormControlLabel,
  FormHelperText,
  IconButton,
  InputAdornment,
  LinearProgress,
  MenuItem,
  Paper,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import {
  MdTranslate,
  MdVisibility,
  MdVisibilityOff,
  MdCheckCircle,
  MdError,
  MdPlayArrow,
  MdRefresh,
  MdDelete,
  MdWarning,
  MdSearch,
  MdStop,
  MdSync,
} from 'react-icons/md'
import httpClient from '../dataProvider/httpClient'

const useStyles = makeStyles((theme) => ({
  root: {
    marginTop: theme.spacing(2),
    marginBottom: theme.spacing(4),
    maxWidth: 860,
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
    fontSize: 24,
    flexShrink: 0,
  },
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
  content: {
    padding: theme.spacing(3),
  },
  section: {
    marginBottom: theme.spacing(3),
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
  field: {
    marginTop: theme.spacing(1.5),
    width: '100%',
  },
  row: {
    display: 'flex',
    gap: theme.spacing(2),
    flexWrap: 'wrap',
    '& > *': {
      flex: '1 1 240px',
    },
  },
  engineCard: {
    padding: theme.spacing(1.5),
    borderRadius: theme.shape.borderRadius,
    background: theme.palette.action.hover,
    marginTop: theme.spacing(1),
    marginBottom: theme.spacing(1.5),
    fontSize: '0.82rem',
    color: theme.palette.text.secondary,
    lineHeight: 1.6,
  },
  testPanel: {
    padding: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
    border: `1px dashed ${theme.palette.divider}`,
    background:
      theme.palette.type === 'dark'
        ? 'rgba(255, 255, 255, 0.02)'
        : 'rgba(0, 0, 0, 0.01)',
    marginTop: theme.spacing(2),
  },
  testResultSuccess: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: theme.spacing(1),
    padding: theme.spacing(1.5),
    borderRadius: theme.shape.borderRadius,
    backgroundColor:
      theme.palette.type === 'dark' ? 'rgba(76, 175, 80, 0.15)' : '#e8f5e9',
    color: theme.palette.type === 'dark' ? '#81c784' : '#2e7d32',
    marginTop: theme.spacing(1.5),
    fontSize: '0.88rem',
  },
  testResultError: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: theme.spacing(1),
    padding: theme.spacing(1.5),
    borderRadius: theme.shape.borderRadius,
    backgroundColor:
      theme.palette.type === 'dark' ? 'rgba(244, 67, 54, 0.15)' : '#ffebee',
    color: theme.palette.type === 'dark' ? '#e57373' : '#c62828',
    marginTop: theme.spacing(1.5),
    fontSize: '0.88rem',
  },
  actions: {
    display: 'flex',
    gap: theme.spacing(2),
    marginTop: theme.spacing(3),
    paddingTop: theme.spacing(2),
    borderTop: `1px solid ${theme.palette.divider}`,
  },
  loadingContainer: {
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    minHeight: 240,
  },
  cacheSection: {
    marginTop: theme.spacing(4),
    paddingTop: theme.spacing(3),
    borderTop: `1px solid ${theme.palette.divider}`,
  },
  batchProgressCard: {
    padding: theme.spacing(2),
    marginBottom: theme.spacing(2.5),
    borderRadius: theme.shape.borderRadius,
    backgroundColor:
      theme.palette.type === 'dark'
        ? 'rgba(33, 150, 243, 0.08)'
        : 'rgba(33, 150, 243, 0.05)',
    border: `1px solid ${theme.palette.primary.main}`,
  },
  tableWrapper: {
    maxHeight: 400,
    marginTop: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
  },
  dialogWarningBox: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1.5),
    padding: theme.spacing(1.5),
    borderRadius: theme.shape.borderRadius,
    backgroundColor:
      theme.palette.type === 'dark' ? 'rgba(255, 152, 0, 0.15)' : '#fff3e0',
    color: theme.palette.type === 'dark' ? '#ffb74d' : '#e65100',
    marginBottom: theme.spacing(2),
  },
  cacheHeaderRow: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    flexWrap: 'wrap',
    gap: theme.spacing(2),
    marginBottom: theme.spacing(2),
  },
  cacheActionButtons: {
    display: 'flex',
    gap: theme.spacing(1.5),
    alignItems: 'center',
    flexWrap: 'wrap',
  },
  singleSearchBox: {
    display: 'flex',
    gap: theme.spacing(1.5),
    alignItems: 'center',
    marginTop: theme.spacing(2),
    marginBottom: theme.spacing(2),
    flexWrap: 'wrap',
  },
}))

const ENGINE_OPTIONS = [
  {
    id: 'gemini',
    label:
      'Google Gemini (支持 gemini-flash-latest / gemini-flash-lite-latest)',
  },
  { id: 'zhipu', label: '智谱 GLM (glm-4-flash，免费高效)' },
  { id: 'baidu', label: '百度翻译 (传统 API)' },
  { id: 'google', label: 'Google Translate (免费公共接口)' },
  { id: 'openai', label: 'OpenAI 兼容接口 (DeepSeek、Groq、自建 API)' },
]

const GEMINI_MODEL_OPTIONS = [
  {
    id: 'gemini-flash-latest',
    label: 'gemini-flash-latest (最新 Flash 模型，效果最好，推荐)',
  },
  {
    id: 'gemini-flash-lite-latest',
    label: 'gemini-flash-lite-latest (最新 Flash-Lite 模型，响应最快更省额度)',
  },
]

const ENGINE_NOTES = {
  gemini:
    'Key 来自 Google AI Studio (aistudio.google.com)。免费配额充足，支持最新的 gemini-flash-latest 与 gemini-flash-lite-latest 模型。若在国内服务器部署且直连受阻，可填写下方的 HTTP 代理或 Base URL。',
  zhipu:
    'Key 来自智谱开放平台 (open.bigmodel.cn)。默认使用 glm-4-flash 模型，调用速度极快且完全免费。',
  baidu:
    '需要百度翻译开放平台 (fanyi.baidu.com/api) 的 App ID 与密钥 (Secret Key)。',
  google: '使用 Google 免费公共翻译接口，无需 API Key，但受限于公共请求频率。',
  openai:
    '适用于所有兼容 OpenAI 格式的服务（例如 DeepSeek、Moonshot、Groq、Ollama、OneAPI 等），需填写 API Base URL 与 API Key。',
}

const LANG_OPTIONS = [
  { id: 'zh-CN', label: '简体中文 (zh-CN)' },
  { id: 'zh-TW', label: '繁體中文 (zh-TW)' },
  { id: 'en', label: 'English (en)' },
  { id: 'ja', label: '日本語 (ja)' },
  { id: 'ko', label: '한국어 (ko)' },
  { id: 'fr', label: 'Français (fr)' },
  { id: 'de', label: 'Deutsch (de)' },
  { id: 'es', label: 'Español (es)' },
]

const emptyConfig = {
  enabled: false,
  engine: 'gemini',
  model: 'gemini-flash-latest',
  apiKey: '',
  secretKey: '',
  appId: '',
  targetLanguage: 'zh-CN',
  proxyUrl: '',
  baseUrl: '',
}

const LyricsTranslation = () => {
  const translate = useTranslate()
  const notify = useNotify()
  const classes = useStyles()

  const [cfg, setCfg] = useState(emptyConfig)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [showKey, setShowKey] = useState(false)

  // Test translation state
  const [testText, setTestText] = useState(
    'Every night in my dreams, I see you, I feel you.',
  )
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState(null)

  useEffect(() => {
    let mounted = true
    httpClient('/api/lyrics/translation/config')
      .then((res) => {
        if (mounted && res.json) {
          setCfg((prev) => ({ ...prev, ...res.json }))
        }
      })
      .catch((err) => {
        notify(
          translate('menu.lyricsTranslation.loadFailed', {
            _: '加载配置失败，请确认是否具备管理员权限',
          }),
          { type: 'warning' },
        )
      })
      .finally(() => {
        if (mounted) setLoading(false)
      })

    return () => {
      mounted = false
    }
  }, [notify, translate])

  const handleChange = (field) => (e) => {
    const val = e.target.type === 'checkbox' ? e.target.checked : e.target.value
    setCfg((prev) => {
      const next = { ...prev, [field]: val }
      if (field === 'engine') {
        if (val === 'gemini') next.model = 'gemini-flash-latest'
        else if (val === 'zhipu') next.model = 'glm-4-flash'
        else if (val === 'openai') next.model = 'gpt-4o-mini'
        else next.model = ''
      }
      return next
    })
    setTestResult(null)
  }

  const handleSave = () => {
    setSaving(true)
    httpClient('/api/lyrics/translation/config', {
      method: 'PUT',
      body: JSON.stringify(cfg),
    })
      .then(() => {
        notify(
          translate('menu.lyricsTranslation.saved', {
            _: '歌词翻译配置已成功保存！',
          }),
          { type: 'success' },
        )
        // Refresh masked values from server
        return httpClient('/api/lyrics/translation/config')
      })
      .then((res) => {
        if (res?.json) {
          setCfg((prev) => ({ ...prev, ...res.json }))
        }
      })
      .catch((err) => {
        notify(
          translate('menu.lyricsTranslation.saveFailed', {
            _: `保存失败: ${err?.message || '请检查权限或网络'}`,
          }),
          { type: 'error' },
        )
      })
      .finally(() => setSaving(false))
  }

  const handleTest = () => {
    setTesting(true)
    setTestResult(null)

    httpClient('/api/lyrics/translation/test', {
      method: 'POST',
      body: JSON.stringify({
        config: cfg,
        sampleText: testText,
      }),
    })
      .then((res) => {
        if (res.json?.success) {
          setTestResult({
            success: true,
            text: res.json.result,
          })
        } else {
          setTestResult({
            success: false,
            error: res.json?.error || '未知错误',
          })
        }
      })
      .catch((err) => {
        setTestResult({
          success: false,
          error:
            err?.message ||
            err?.body?.error ||
            '请求失败，请检查网络或服务配置',
        })
      })
      .finally(() => setTesting(false))
  }

  // Cache and retranslation states
  const [cachedList, setCachedList] = useState([])
  const [loadingCache, setLoadingCache] = useState(false)
  const [retranslatingId, setRetranslatingId] = useState(null)
  const [retranslateAllDialogOpen, setRetranslateAllDialogOpen] =
    useState(false)
  const [clearAllDialogOpen, setClearAllDialogOpen] = useState(false)
  const [filterText, setFilterText] = useState('')
  const [singleInputId, setSingleInputId] = useState('')
  const [batchStatus, setBatchStatus] = useState({
    running: false,
    total: 0,
    processed: 0,
    success: 0,
    failed: 0,
    current: '',
    lastError: '',
  })

  const fetchCacheList = useCallback(() => {
    setLoadingCache(true)
    httpClient('/api/lyrics/translation/cache')
      .then((res) => {
        if (res.json?.items) {
          setCachedList(res.json.items)
        }
      })
      .catch(() => {})
      .finally(() => setLoadingCache(false))
  }, [])

  useEffect(() => {
    fetchCacheList()
    httpClient('/api/lyrics/translation/retranslate-status')
      .then((res) => {
        if (res.json?.running) {
          setBatchStatus(res.json)
        }
      })
      .catch(() => {})
  }, [fetchCacheList])

  useEffect(() => {
    if (!batchStatus.running) return

    const timer = setInterval(() => {
      httpClient('/api/lyrics/translation/retranslate-status')
        .then((res) => {
          if (res.json) {
            setBatchStatus(res.json)
            if (!res.json.running) {
              clearInterval(timer)
              fetchCacheList()
              notify(
                translate('menu.lyricsTranslation.batchFinished', {
                  _: `所有歌曲重新翻译完成！成功 ${res.json.success} 首，失败 ${res.json.failed} 首。`,
                }),
                { type: 'success', autoHideDuration: 4000 },
              )
            }
          }
        })
        .catch(() => {})
    }, 1000)

    return () => clearInterval(timer)
  }, [batchStatus.running, fetchCacheList, notify, translate])

  const handleRetranslateSingle = async (songId) => {
    if (!songId || retranslatingId) return
    setRetranslatingId(songId)
    notify(
      translate('menu.lyricsTranslation.retranslatingSingle', {
        _: '正在使用当前选定的模型重新翻译该歌曲...',
      }),
      { type: 'info', autoHideDuration: 2500 },
    )
    try {
      await httpClient('/api/lyrics/translate', {
        method: 'POST',
        body: JSON.stringify({
          songId,
          targetLang: cfg.targetLanguage,
          force: true,
        }),
      })
      notify(
        translate('menu.lyricsTranslation.retranslateSuccess', {
          _: '歌曲重新翻译成功并已更新缓存！',
        }),
        { type: 'success', autoHideDuration: 3000 },
      )
      fetchCacheList()
    } catch (err) {
      notify(
        translate('menu.lyricsTranslation.retranslateFailed', {
          _: `重新翻译失败: ${err?.message || '请检查模型或网络配置'}`,
        }),
        { type: 'error' },
      )
    } finally {
      setRetranslatingId(null)
    }
  }

  const handleDeleteSingle = async (songId) => {
    try {
      await httpClient(`/api/lyrics/translation/cache/${songId}`, {
        method: 'DELETE',
      })
      notify(
        translate('menu.lyricsTranslation.deleteCacheSuccess', {
          _: '已删除该歌曲的翻译缓存',
        }),
        { type: 'info', autoHideDuration: 2000 },
      )
      fetchCacheList()
    } catch (err) {
      notify(
        translate('menu.lyricsTranslation.deleteCacheFailed', {
          _: `删除失败: ${err?.message || '未知错误'}`,
        }),
        { type: 'error' },
      )
    }
  }

  const handleConfirmRetranslateAll = async () => {
    setRetranslateAllDialogOpen(false)
    try {
      await httpClient('/api/lyrics/translation/retranslate-all', {
        method: 'POST',
      })
      notify(
        translate('menu.lyricsTranslation.batchStarted', {
          _: '已启动后台批量重新翻译任务，正在处理中...',
        }),
        { type: 'info', autoHideDuration: 3000 },
      )
      setBatchStatus((prev) => ({ ...prev, running: true }))
    } catch (err) {
      notify(
        translate('menu.lyricsTranslation.batchStartFailed', {
          _: `启动失败: ${err?.message || '未知错误'}`,
        }),
        { type: 'error' },
      )
    }
  }

  const handleCancelBatch = async () => {
    try {
      await httpClient('/api/lyrics/translation/retranslate-cancel', {
        method: 'POST',
      })
      notify(
        translate('menu.lyricsTranslation.batchCanceled', {
          _: '已请求终止批量重新翻译任务',
        }),
        { type: 'info' },
      )
      setBatchStatus((prev) => ({ ...prev, running: false }))
      fetchCacheList()
    } catch (err) {
      notify(err.message || 'Error', 'warning')
    }
  }

  const handleConfirmClearAll = async () => {
    setClearAllDialogOpen(false)
    try {
      const res = await httpClient('/api/lyrics/translation/cache', {
        method: 'DELETE',
      })
      notify(
        translate('menu.lyricsTranslation.clearedAll', {
          _: `已成功清空全部翻译缓存（共清除 ${res.json?.cleared || 0} 个文件）！`,
        }),
        { type: 'success', autoHideDuration: 3000 },
      )
      fetchCacheList()
    } catch (err) {
      notify(
        translate('menu.lyricsTranslation.clearFailed', {
          _: `清空失败: ${err?.message || '未知错误'}`,
        }),
        { type: 'error' },
      )
    }
  }

  const filteredList = cachedList.filter((item) => {
    if (!filterText) return true
    const q = filterText.toLowerCase()
    return (
      (item.title && item.title.toLowerCase().includes(q)) ||
      (item.artist && item.artist.toLowerCase().includes(q)) ||
      (item.songId && item.songId.toLowerCase().includes(q)) ||
      (item.model && item.model.toLowerCase().includes(q)) ||
      (item.engine && item.engine.toLowerCase().includes(q))
    )
  })

  const isBaidu = cfg.engine === 'baidu'
  const isGemini = cfg.engine === 'gemini'
  const isOpenAI = cfg.engine === 'openai'
  const isZhipu = cfg.engine === 'zhipu'
  const needsApiKey = cfg.engine !== 'google'

  if (loading) {
    return (
      <Card className={classes.root}>
        <Box className={classes.loadingContainer}>
          <CircularProgress />
        </Box>
      </Card>
    )
  }

  return (
    <Card className={classes.root}>
      <Title
        title={
          'Navidrome - ' +
          translate('menu.lyricsTranslation.name', { _: '歌词翻译' })
        }
      />

      {/* Header */}
      <Box className={classes.header}>
        <Box className={classes.headerIcon}>
          <MdTranslate />
        </Box>
        <Box>
          <Typography className={classes.title}>
            {translate('menu.lyricsTranslation.title', { _: '歌词双语翻译' })}
          </Typography>
          <Typography className={classes.subtitle}>
            {translate('menu.lyricsTranslation.description', {
              _: '配置多语言歌词双语翻译引擎。仅在播放器或第三方客户端按需点击翻译时才实时触发，不增加后台扫描负担与性能开销。',
            })}
          </Typography>
        </Box>
      </Box>

      <CardContent className={classes.content}>
        {/* Enable / Disable switch */}
        <Box className={classes.section}>
          <FormControlLabel
            control={
              <Switch
                id="lyrics-translation-enabled"
                color="primary"
                checked={!!cfg.enabled}
                onChange={handleChange('enabled')}
              />
            }
            label={
              <Typography style={{ fontWeight: 600 }}>
                {translate('menu.lyricsTranslation.enable', {
                  _: '启用歌词翻译功能',
                })}
              </Typography>
            }
          />
          <FormHelperText>
            {translate('menu.lyricsTranslation.enableHint', {
              _: '开启后，播放器工具栏将显示「翻译歌词」按钮，点击即可实时获取双语对照歌词。',
            })}
          </FormHelperText>
        </Box>

        <Collapse in={!!cfg.enabled}>
          <Divider style={{ marginBottom: 24 }} />

          {/* Engine Selection */}
          <Box className={classes.section}>
            <Typography className={classes.sectionTitle}>
              {translate('menu.lyricsTranslation.engineSection', {
                _: '翻译引擎与语言',
              })}
            </Typography>

            <Box className={classes.row}>
              <TextField
                select
                label={translate('menu.lyricsTranslation.engine', {
                  _: '翻译引擎',
                })}
                value={cfg.engine || 'gemini'}
                onChange={handleChange('engine')}
                variant="outlined"
                size="small"
                className={classes.field}
              >
                {ENGINE_OPTIONS.map((o) => (
                  <MenuItem key={o.id} value={o.id}>
                    {o.label}
                  </MenuItem>
                ))}
              </TextField>

              <TextField
                select
                label={translate('menu.lyricsTranslation.targetLanguage', {
                  _: '翻译目标语言',
                })}
                value={cfg.targetLanguage || 'zh-CN'}
                onChange={handleChange('targetLanguage')}
                variant="outlined"
                size="small"
                className={classes.field}
              >
                {LANG_OPTIONS.map((o) => (
                  <MenuItem key={o.id} value={o.id}>
                    {o.label}
                  </MenuItem>
                ))}
              </TextField>
            </Box>

            {/* Engine Description Note */}
            {cfg.engine && ENGINE_NOTES[cfg.engine] && (
              <Box className={classes.engineCard}>
                {ENGINE_NOTES[cfg.engine]}
              </Box>
            )}

            {/* Gemini Model Selector */}
            {isGemini && (
              <TextField
                select
                label={translate('menu.lyricsTranslation.model', {
                  _: 'Gemini 模型',
                })}
                value={cfg.model || 'gemini-flash-latest'}
                onChange={handleChange('model')}
                variant="outlined"
                size="small"
                className={classes.field}
                helperText="支持 Google Gemini 最新发布的 flash 与 flash-lite 模型"
              >
                {GEMINI_MODEL_OPTIONS.map((o) => (
                  <MenuItem key={o.id} value={o.id}>
                    {o.label}
                  </MenuItem>
                ))}
              </TextField>
            )}

            {/* Zhipu Model Field */}
            {isZhipu && (
              <TextField
                label={translate('menu.lyricsTranslation.model', {
                  _: '智谱模型',
                })}
                value={cfg.model || 'glm-4-flash'}
                onChange={handleChange('model')}
                variant="outlined"
                size="small"
                className={classes.field}
                helperText="默认使用 glm-4-flash (完全免费调用)"
              />
            )}

            {/* OpenAI Model Field */}
            {isOpenAI && (
              <TextField
                label={translate('menu.lyricsTranslation.model', {
                  _: '模型名称 (Model)',
                })}
                value={cfg.model || 'gpt-4o-mini'}
                onChange={handleChange('model')}
                variant="outlined"
                size="small"
                className={classes.field}
                helperText="例如 gpt-4o-mini、deepseek-chat、qwen-plus 等"
              />
            )}
          </Box>

          <Divider style={{ marginBottom: 24 }} />

          {/* Credentials */}
          <Box className={classes.section}>
            <Typography className={classes.sectionTitle}>
              {translate('menu.lyricsTranslation.credentialsSection', {
                _: 'API 密钥与网络参数',
              })}
            </Typography>

            {/* API Key */}
            {needsApiKey && !isBaidu && (
              <TextField
                label={translate('menu.lyricsTranslation.apiKey', {
                  _: 'API Key',
                })}
                value={cfg.apiKey || ''}
                onChange={handleChange('apiKey')}
                variant="outlined"
                size="small"
                type={showKey ? 'text' : 'password'}
                autoComplete="new-password"
                className={classes.field}
                helperText={translate('menu.lyricsTranslation.apiKeyHint', {
                  _: '已保存时将显示脱敏值 (如 sk-****abcd)，若需修改直接输入新值覆盖保存即可。',
                })}
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        aria-label="toggle password visibility"
                        onClick={() => setShowKey(!showKey)}
                        edge="end"
                        size="small"
                      >
                        {showKey ? <MdVisibilityOff /> : <MdVisibility />}
                      </IconButton>
                    </InputAdornment>
                  ),
                }}
              />
            )}

            {/* Baidu App ID & Secret Key */}
            {isBaidu && (
              <Box className={classes.row}>
                <TextField
                  label={translate('menu.lyricsTranslation.appId', {
                    _: '百度 App ID',
                  })}
                  value={cfg.appId || ''}
                  onChange={handleChange('appId')}
                  variant="outlined"
                  size="small"
                  className={classes.field}
                />
                <TextField
                  label={translate('menu.lyricsTranslation.secretKey', {
                    _: '百度密钥 (Secret Key)',
                  })}
                  value={cfg.secretKey || ''}
                  onChange={handleChange('secretKey')}
                  variant="outlined"
                  size="small"
                  type={showKey ? 'text' : 'password'}
                  autoComplete="new-password"
                  className={classes.field}
                  InputProps={{
                    endAdornment: (
                      <InputAdornment position="end">
                        <IconButton
                          aria-label="toggle password visibility"
                          onClick={() => setShowKey(!showKey)}
                          edge="end"
                          size="small"
                        >
                          {showKey ? <MdVisibilityOff /> : <MdVisibility />}
                        </IconButton>
                      </InputAdornment>
                    ),
                  }}
                />
              </Box>
            )}

            {/* OpenAI / Custom Base URL */}
            {(isOpenAI || isGemini) && (
              <TextField
                label={translate('menu.lyricsTranslation.baseUrl', {
                  _: isGemini
                    ? '自定义 Base URL (反向代理，可选)'
                    : 'API Base URL (例如 https://api.deepseek.com/v1)',
                })}
                value={cfg.baseUrl || ''}
                onChange={handleChange('baseUrl')}
                variant="outlined"
                size="small"
                className={classes.field}
                placeholder={
                  isGemini
                    ? 'https://generativelanguage.googleapis.com'
                    : 'https://api.openai.com/v1'
                }
                helperText={
                  isGemini
                    ? '直连受阻或使用了 Gemini 反向代理时填写，留空默认官方地址。'
                    : 'OpenAI 兼容接口必填服务商提供的 Base URL。'
                }
              />
            )}

            {/* Proxy URL */}
            <TextField
              label={translate('menu.lyricsTranslation.proxyUrl', {
                _: 'HTTP / SOCKS5 代理 (可选)',
              })}
              value={cfg.proxyUrl || ''}
              onChange={handleChange('proxyUrl')}
              variant="outlined"
              size="small"
              className={classes.field}
              placeholder="http://127.0.0.1:7890"
              helperText={translate('menu.lyricsTranslation.proxyHint', {
                _: '例如 http://127.0.0.1:7890。仅在服务器环境无法直连外部翻译服务时填写，留空使用系统默认网络。',
              })}
            />
          </Box>

          <Divider style={{ marginBottom: 24 }} />

          {/* Test Panel */}
          <Box className={classes.section}>
            <Typography className={classes.sectionTitle}>
              {translate('menu.lyricsTranslation.testSection', {
                _: '连接与翻译测试',
              })}
            </Typography>

            <Paper className={classes.testPanel} elevation={0}>
              <TextField
                label={translate('menu.lyricsTranslation.testInput', {
                  _: '待测试翻译文本',
                })}
                value={testText}
                onChange={(e) => setTestText(e.target.value)}
                variant="outlined"
                size="small"
                fullWidth
              />

              <Box mt={2} display="flex" alignItems="center" gap={2}>
                <Button
                  variant="outlined"
                  color="primary"
                  size="small"
                  onClick={handleTest}
                  disabled={testing}
                  startIcon={
                    testing ? <CircularProgress size={16} /> : <MdPlayArrow />
                  }
                >
                  {testing
                    ? translate('menu.lyricsTranslation.testing', {
                        _: '正在测试...',
                      })
                    : translate('menu.lyricsTranslation.testBtn', {
                        _: '立即测试翻译',
                      })}
                </Button>
                <Typography variant="caption" color="textSecondary">
                  测试会直接调用上方配置的引擎与密钥验证连通性
                </Typography>
              </Box>

              {testResult && testResult.success && (
                <Box className={classes.testResultSuccess}>
                  <MdCheckCircle
                    size={20}
                    style={{ flexShrink: 0, marginTop: 2 }}
                  />
                  <Box>
                    <Typography variant="body2" style={{ fontWeight: 600 }}>
                      {translate('menu.lyricsTranslation.testSuccess', {
                        _: '测试成功！翻译结果：',
                      })}
                    </Typography>
                    <Typography variant="body2" style={{ marginTop: 4 }}>
                      {testResult.text}
                    </Typography>
                  </Box>
                </Box>
              )}

              {testResult && !testResult.success && (
                <Box className={classes.testResultError}>
                  <MdError size={20} style={{ flexShrink: 0, marginTop: 2 }} />
                  <Box>
                    <Typography variant="body2" style={{ fontWeight: 600 }}>
                      {translate('menu.lyricsTranslation.testFailed', {
                        _: '测试失败：',
                      })}
                    </Typography>
                    <Typography
                      variant="body2"
                      style={{ marginTop: 4, wordBreak: 'break-all' }}
                    >
                      {testResult.error}
                    </Typography>
                  </Box>
                </Box>
              )}
            </Paper>
          </Box>
        </Collapse>

        {/* Action Buttons */}
        <Box className={classes.actions}>
          <Button
            variant="contained"
            color="primary"
            onClick={handleSave}
            disabled={saving}
            data-testid="save-lyrics-translation-button"
            startIcon={
              saving ? <CircularProgress size={18} color="inherit" /> : null
            }
          >
            {saving
              ? translate('menu.lyricsTranslation.saving', { _: '正在保存...' })
              : translate('menu.lyricsTranslation.save', { _: '保存配置' })}
          </Button>
        </Box>

        {/* Divider and Lyrics Cache & Retranslation Section */}
        <Box className={classes.cacheSection}>
          <Box className={classes.cacheHeaderRow}>
            <Box>
              <Typography className={classes.sectionTitle}>
                <MdSync size={20} color="#1976d2" />
                {translate('menu.lyricsTranslation.cacheManagementTitle', {
                  _: '已翻译歌曲管理与重新翻译',
                })}
              </Typography>
              <Typography variant="body2" color="textSecondary">
                {translate('menu.lyricsTranslation.cacheManagementDesc', {
                  _: '已翻译的歌词默认永久缓存于本地磁盘。当您切换了不同的 AI 翻译引擎或高精度模型后，可在此重新翻译单首歌曲或批量重新翻译所有歌曲。',
                })}
              </Typography>
            </Box>

            <Box className={classes.cacheActionButtons}>
              <Chip
                label={`已缓存: ${cachedList.length} 首`}
                color="primary"
                variant="outlined"
                size="small"
              />
              <Button
                variant="contained"
                color="primary"
                disabled={cachedList.length === 0 || batchStatus.running}
                onClick={() => setRetranslateAllDialogOpen(true)}
                startIcon={<MdRefresh />}
                size="small"
              >
                {translate('menu.lyricsTranslation.retranslateAllBtn', {
                  _: '重新翻译所有歌曲',
                })}
              </Button>
              <Button
                variant="outlined"
                color="secondary"
                disabled={cachedList.length === 0 || batchStatus.running}
                onClick={() => setClearAllDialogOpen(true)}
                startIcon={<MdDelete />}
                size="small"
              >
                {translate('menu.lyricsTranslation.clearAllCacheBtn', {
                  _: '清空全部缓存',
                })}
              </Button>
              <Tooltip title="刷新缓存列表">
                <IconButton
                  size="small"
                  onClick={fetchCacheList}
                  disabled={loadingCache}
                >
                  {loadingCache ? (
                    <CircularProgress size={16} />
                  ) : (
                    <MdRefresh size={18} />
                  )}
                </IconButton>
              </Tooltip>
            </Box>
          </Box>

          {/* Batch Retranslation Progress Card */}
          {batchStatus.running && (
            <Paper className={classes.batchProgressCard} elevation={0}>
              <Box
                display="flex"
                justifyContent="space-between"
                alignItems="center"
                mb={1}
              >
                <Typography
                  variant="subtitle2"
                  style={{
                    fontWeight: 600,
                    display: 'flex',
                    alignItems: 'center',
                    gap: 6,
                  }}
                >
                  <CircularProgress size={16} />
                  {translate('menu.lyricsTranslation.batchRunningTitle', {
                    _: '正在批量重新翻译所有已缓存歌曲...',
                  })}
                </Typography>
                <Button
                  size="small"
                  color="secondary"
                  variant="outlined"
                  startIcon={<MdStop />}
                  onClick={handleCancelBatch}
                >
                  终止任务
                </Button>
              </Box>

              <LinearProgress
                variant="determinate"
                value={
                  batchStatus.total > 0
                    ? Math.round(
                        (batchStatus.processed / batchStatus.total) * 100,
                      )
                    : 0
                }
                style={{ height: 8, borderRadius: 4, marginBottom: 8 }}
              />

              <Box
                display="flex"
                justifyContent="space-between"
                alignItems="center"
              >
                <Typography variant="body2" color="textSecondary">
                  当前进度: {batchStatus.processed} / {batchStatus.total} (
                  {batchStatus.total > 0
                    ? Math.round(
                        (batchStatus.processed / batchStatus.total) * 100,
                      )
                    : 0}
                  %)
                  {batchStatus.current
                    ? ` — 处理中: ${batchStatus.current}`
                    : ''}
                </Typography>
                <Typography variant="body2" color="textSecondary">
                  成功:{' '}
                  <span style={{ color: '#4caf50', fontWeight: 600 }}>
                    {batchStatus.success}
                  </span>{' '}
                  | 失败:{' '}
                  <span style={{ color: '#f44336', fontWeight: 600 }}>
                    {batchStatus.failed}
                  </span>
                </Typography>
              </Box>
            </Paper>
          )}

          {/* Single Song Quick Retranslate Input */}
          <Box className={classes.singleSearchBox}>
            <TextField
              variant="outlined"
              size="small"
              placeholder="输入歌曲 ID 重新翻译单首歌曲..."
              value={singleInputId}
              onChange={(e) => setSingleInputId(e.target.value)}
              style={{ flex: '1 1 280px' }}
              InputProps={{
                startAdornment: (
                  <InputAdornment position="start">
                    <MdTranslate size={18} color="#888" />
                  </InputAdornment>
                ),
              }}
            />
            <Button
              variant="contained"
              color="default"
              size="small"
              disabled={!singleInputId.trim() || !!retranslatingId}
              onClick={() => handleRetranslateSingle(singleInputId.trim())}
              startIcon={
                retranslatingId === singleInputId.trim() ? (
                  <CircularProgress size={16} />
                ) : (
                  <MdRefresh />
                )
              }
            >
              重新翻译此歌曲
            </Button>
          </Box>

          {/* Cached Songs Table */}
          <Box mt={2}>
            <Box
              display="flex"
              justifyContent="space-between"
              alignItems="center"
              mb={1}
            >
              <Typography variant="subtitle2" style={{ fontWeight: 600 }}>
                已缓存歌曲列表 ({filteredList.length})
              </Typography>
              <TextField
                variant="outlined"
                size="small"
                placeholder="搜索歌名、歌手或模型..."
                value={filterText}
                onChange={(e) => setFilterText(e.target.value)}
                style={{ width: 220 }}
                InputProps={{
                  startAdornment: (
                    <InputAdornment position="start">
                      <MdSearch size={18} color="#888" />
                    </InputAdornment>
                  ),
                }}
              />
            </Box>

            {loadingCache ? (
              <Box display="flex" justifyContent="center" py={4}>
                <CircularProgress size={28} />
              </Box>
            ) : cachedList.length === 0 ? (
              <Paper
                style={{ padding: 24, textAlign: 'center', color: '#888' }}
                elevation={0}
              >
                暂无已缓存的翻译歌曲。在播放器中点击「翻译歌词」后，结果将自动缓存在这里。
              </Paper>
            ) : (
              <TableContainer
                component={Paper}
                className={classes.tableWrapper}
                elevation={0}
              >
                <Table size="small" stickyHeader>
                  <TableHead>
                    <TableRow>
                      <TableCell>歌曲</TableCell>
                      <TableCell>语言</TableCell>
                      <TableCell>翻译模型</TableCell>
                      <TableCell>更新时间</TableCell>
                      <TableCell align="right">操作</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {filteredList.map((item) => {
                      const isRetranslating = retranslatingId === item.songId
                      return (
                        <TableRow
                          key={`${item.songId}_${item.targetLang}`}
                          hover
                        >
                          <TableCell>
                            <Typography
                              variant="body2"
                              style={{ fontWeight: 600 }}
                            >
                              {item.title || item.songId}
                            </Typography>
                            {item.artist && (
                              <Typography
                                variant="caption"
                                color="textSecondary"
                              >
                                {item.artist}
                              </Typography>
                            )}
                          </TableCell>
                          <TableCell>
                            <Chip
                              label={item.targetLang}
                              size="small"
                              variant="outlined"
                            />
                          </TableCell>
                          <TableCell>
                            <Typography
                              variant="caption"
                              style={{ fontFamily: 'monospace' }}
                            >
                              {item.engine}{' '}
                              {item.model ? `(${item.model})` : ''}
                            </Typography>
                          </TableCell>
                          <TableCell>
                            <Typography variant="caption" color="textSecondary">
                              {item.updatedAt
                                ? new Date(item.updatedAt).toLocaleString()
                                : '-'}
                            </Typography>
                          </TableCell>
                          <TableCell align="right">
                            <Tooltip title="使用当前选定模型重新翻译此歌曲">
                              <span>
                                <IconButton
                                  size="small"
                                  color="primary"
                                  disabled={
                                    isRetranslating || batchStatus.running
                                  }
                                  onClick={() =>
                                    handleRetranslateSingle(item.songId)
                                  }
                                >
                                  {isRetranslating ? (
                                    <CircularProgress size={16} />
                                  ) : (
                                    <MdRefresh size={18} />
                                  )}
                                </IconButton>
                              </span>
                            </Tooltip>
                            <Tooltip title="删除该歌曲翻译缓存">
                              <span>
                                <IconButton
                                  size="small"
                                  color="secondary"
                                  disabled={
                                    isRetranslating || batchStatus.running
                                  }
                                  onClick={() =>
                                    handleDeleteSingle(item.songId)
                                  }
                                >
                                  <MdDelete size={18} />
                                </IconButton>
                              </span>
                            </Tooltip>
                          </TableCell>
                        </TableRow>
                      )
                    })}
                  </TableBody>
                </Table>
              </TableContainer>
            )}
          </Box>
        </Box>

        {/* Retranslate All Confirmation Dialog */}
        <Dialog
          open={retranslateAllDialogOpen}
          onClose={() => setRetranslateAllDialogOpen(false)}
          aria-labelledby="retranslate-all-dialog-title"
        >
          <DialogTitle
            id="retranslate-all-dialog-title"
            style={{ display: 'flex', alignItems: 'center', gap: 8 }}
          >
            <MdWarning color="#ff9800" size={24} />
            {translate('menu.lyricsTranslation.retranslateAllTitle', {
              _: '警告：确认重新翻译所有歌曲？',
            })}
          </DialogTitle>
          <DialogContent>
            <Box className={classes.dialogWarningBox}>
              <Typography variant="body2" style={{ fontWeight: 600 }}>
                当前生效模型：{cfg.engine} ({cfg.model || '默认'})
              </Typography>
            </Box>
            <DialogContentText>
              您即将对已缓存的全部 <strong>{cachedList.length}</strong>{' '}
              首歌曲发起重新翻译。
            </DialogContentText>
            <DialogContentText>
              ⚠️ <strong>注意事项：</strong>
              <br />
              1. 此操作将覆盖本地现有的全部翻译缓存；
              <br />
              2. 系统将在后台逐一调用外部 AI
              接口重新翻译，根据歌曲数量可能消耗较多 API 配额并需要数分钟；
              <br />
              3. 重新翻译过程中您随时可以在本页面点击终止任务。
            </DialogContentText>
            <DialogContentText
              style={{ fontWeight: 600, color: '#f44336', marginTop: 16 }}
            >
              确定要立即重新翻译所有歌曲吗？
            </DialogContentText>
          </DialogContent>
          <DialogActions style={{ padding: '16px 24px' }}>
            <Button
              onClick={() => setRetranslateAllDialogOpen(false)}
              color="default"
            >
              取消
            </Button>
            <Button
              onClick={handleConfirmRetranslateAll}
              variant="contained"
              color="secondary"
              style={{ backgroundColor: '#f44336', color: '#fff' }}
              startIcon={<MdRefresh />}
            >
              确认重新翻译全部歌曲
            </Button>
          </DialogActions>
        </Dialog>

        {/* Clear All Confirmation Dialog */}
        <Dialog
          open={clearAllDialogOpen}
          onClose={() => setClearAllDialogOpen(false)}
        >
          <DialogTitle
            style={{ display: 'flex', alignItems: 'center', gap: 8 }}
          >
            <MdDelete color="#f44336" size={24} />
            确认清空全部翻译缓存？
          </DialogTitle>
          <DialogContent>
            <DialogContentText>
              清空后，本地磁盘上已保存的全部{' '}
              <strong>{cachedList.length}</strong> 首歌曲翻译文件将被永久删除。
            </DialogContentText>
            <DialogContentText>
              用户之后在播放器中点击翻译时，将自动使用当前配置的新模型重新生成双语歌词。
            </DialogContentText>
          </DialogContent>
          <DialogActions style={{ padding: '16px 24px' }}>
            <Button
              onClick={() => setClearAllDialogOpen(false)}
              color="default"
            >
              取消
            </Button>
            <Button
              onClick={handleConfirmClearAll}
              variant="contained"
              color="secondary"
              style={{ backgroundColor: '#f44336', color: '#fff' }}
            >
              确认清空全部缓存
            </Button>
          </DialogActions>
        </Dialog>
      </CardContent>
    </Card>
  )
}

export default LyricsTranslation
