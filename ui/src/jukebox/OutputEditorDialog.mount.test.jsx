import React from 'react'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ThemeProvider, createTheme } from '@material-ui/core/styles'
import OutputEditorDialog from './OutputEditorDialog'
import en from '../i18n/en.json'

// 这个文件**故意不 mock** ./XiaomiAuthBlock。
// OutputEditorDialog.test.jsx 把它整个 mock 掉了，于是"选中类型 → 真实授权块
// 挂载"这条路径没有测试覆盖：曾经漏传 formData prop，XiaomiAuthBlock 读
// formData.account 直接抛 TypeError，整个向导被 ErrorBoundary 吞成"发生错误"。
// 挂载类崩溃只有在真实组件上才测得出来。
const translateReal = (key, opts = {}) => {
  const path = key.split('.')
  let node = en
  for (const p of path) node = node?.[p]
  return typeof node === 'string' ? node : opts._ || key
}

const postMock = vi.fn()

vi.mock('react-admin', () => ({
  useNotify: () => vi.fn(),
  useTranslate: () => translateReal,
  Title: ({ children }) => <div>{children}</div>,
}))

vi.mock('../dataProvider/httpClient', () => ({
  __esModule: true,
  default: (...args) => postMock(...args),
}))

const renderDialog = (props = {}) =>
  render(
    <ThemeProvider theme={createTheme()}>
      <OutputEditorDialog
        mode="create"
        onClose={vi.fn()}
        onSaved={vi.fn()}
        {...props}
      />
    </ThemeProvider>,
  )

describe('<OutputEditorDialog /> real XiaomiAuthBlock mount', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    postMock.mockResolvedValue({ json: {} })
  })

  it('renders the real auth block without crashing when the xiaomi type is picked', async () => {
    renderDialog()

    const card = await screen.findByText('Xiaomi speaker')
    fireEvent.click(card.closest('button') || card)

    // 之前这里直接抛 TypeError（formData 未传），页面变成"发生错误"
    await waitFor(() =>
      expect(
        screen.getByText('Select the speakers to add'),
      ).toBeInTheDocument(),
    )
    // 真实组件的登录三选项都在
    expect(screen.getByText('米家扫码登录 (推荐)')).toBeInTheDocument()
    expect(screen.getByText('账号密码登录')).toBeInTheDocument()
    expect(screen.getByText('passToken 凭证直填')).toBeInTheDocument()
  })

  it('does not crash on the mpd/dlna types either', async () => {
    for (const type of ['MPD', 'DLNA / UPnP']) {
      const { unmount } = renderDialog()
      const card = await screen.findByText(type)
      fireEvent.click(card.closest('button') || card)
      // 不崩，且进入了连接步骤
      await waitFor(() => expect(screen.getByText('Next')).toBeInTheDocument())
      unmount()
    }
  })

  it('passes formData through so the auth block can seed the account fields', async () => {
    // 编辑已存在的设备时，账号字段应当被已有值预填（formData 传通的表现）
    renderDialog({
      mode: 'edit',
      output: {
        id: 'xiaoai',
        name: '书房音箱',
        type: 'xiaomi',
        address: '192.168.1.10',
        account: '1250258297',
        passToken: 'V1:existing',
      },
    })

    // 编辑模式直接落在连接步骤
    await waitFor(() =>
      expect(
        screen.getByText('Select the speakers to add'),
      ).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByText('passToken 凭证直填'))
    // 已有凭据被带进输入框
    await waitFor(() => {
      const boxes = screen.getAllByRole('textbox')
      const values = boxes.map((b) => b.value)
      expect(values).toContain('1250258297')
      expect(values).toContain('V1:existing')
    })
  })
})
