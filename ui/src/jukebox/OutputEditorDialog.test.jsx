import React from 'react'
import {
  render,
  screen,
  fireEvent,
  waitFor,
  within,
} from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ThemeProvider, createTheme } from '@material-ui/core/styles'
import OutputEditorDialog from './OutputEditorDialog'
import en from '../i18n/en.json'

// 组件里用的是 translate(key, {_: '中文默认'})；测试要走真实语言包，
// 否则断言的 label 会和界面实际显示的对不上。
const translateReal = (key, opts = {}) => {
  const path = key.split('.')
  let node = en
  for (const p of path) node = node?.[p]
  return typeof node === 'string' ? node : opts._ || key
}

// 勾选交互本身在 XiaomiAuthBlock.test.jsx 里覆盖；这里要测的是"向导如何把
// 勾选结果变成 N 份表单、并在保存时逐台创建"，所以直接把勾选结果喂进去。
const mockNotify = vi.fn()
const mockOnSaved = vi.fn()
const postMock = vi.fn()

let pickedDevices = []

vi.mock('./XiaomiAuthBlock', () => ({
  __esModule: true,
  default: ({ onSelectionChange }) => (
    <div>
      <button type="button" onClick={() => onSelectionChange?.(pickedDevices)}>
        模拟勾选
      </button>
      <div data-testid="picked">
        {pickedDevices.map((d) => d.name).join('、')}
      </div>
    </div>
  ),
}))

vi.mock('react-admin', () => ({
  useNotify: () => mockNotify,
  useTranslate: () => (key, opts) => translateReal(key, opts),
  Title: ({ children }) => <div>{children}</div>,
}))

vi.mock('../dataProvider/httpClient', () => ({
  __esModule: true,
  default: (...args) => postMock(...args),
}))

const SPEAKERS = [
  {
    did: 'd1',
    name: '小爱同学一代',
    localip: '192.168.31.142',
    token: 'tk1',
    model: 'xiaomi.wifispeaker.s12',
  },
  {
    did: 'd2',
    name: 'Redmi小爱音箱Play',
    localip: '192.168.31.232',
    token: 'tk2',
    model: 'xiaomi.wifispeaker.l7a',
  },
  {
    did: 'd3',
    name: '小爱音箱Pro',
    localip: '0.0.0.1',
    token: 'tk3',
    model: 'xiaomi.wifispeaker.l05b',
  },
]

const renderDialog = () =>
  render(
    <ThemeProvider theme={createTheme()}>
      <OutputEditorDialog
        mode="create"
        onClose={vi.fn()}
        onSaved={mockOnSaved}
      />
    </ThemeProvider>,
  )

// 选中小爱音箱卡片会自动进入第 2 步（TypePicker 的 onChange 里就 setStep(1)），
// 所以这里不需要再点"下一步"。
const pickXiaomiAndSimulateSelection = async () => {
  const xiaomiCard = await screen.findByText('Xiaomi speaker')
  fireEvent.click(xiaomiCard.closest('button') || xiaomiCard)
  expect(
    await screen.findByText('Select the speakers to add'),
  ).toBeInTheDocument()
  // 授权块把勾选结果交给向导
  fireEvent.click(screen.getByText('模拟勾选'))
  return waitFor(() =>
    expect(screen.getByTestId('picked')).toHaveTextContent('小爱同学一代'),
  )
}

const goToStep3 = async () => {
  const next = screen.getByRole('button', { name: 'Next' })
  // 第 2 步的门槛是"至少勾选一台"
  await waitFor(() => expect(next).toBeEnabled())
  fireEvent.click(next)
  // getAllByText：这个标题和 helper 文案在 DOM 里都可能出现
  await waitFor(() =>
    expect(
      screen.getAllByText(/Per-speaker configuration/i).length,
    ).toBeGreaterThan(0),
  )
}

describe('<OutputEditorDialog /> xiaomi multi-select', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    postMock.mockResolvedValue({ json: {} })
    pickedDevices = SPEAKERS
  })

  it('step 2 has no single-device name/ID/address form', async () => {
    renderDialog()
    await pickXiaomiAndSimulateSelection()

    // 授权块在
    expect(screen.getByTestId('picked')).toHaveTextContent('小爱同学一代')
    // 但没有单台的 名称/ID/地址 输入框
    expect(screen.queryByLabelText('Name')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('ID')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Address')).not.toBeInTheDocument()
  })

  it('renders one editable form per checked device on the next step', async () => {
    renderDialog()
    await pickXiaomiAndSimulateSelection()

    await goToStep3()

    // 三台 -> 三份表单
    const nameInputs = screen.getAllByRole('textbox', { name: 'Name' })
    expect(nameInputs).toHaveLength(3)
    // 预填正确
    expect(nameInputs[0]).toHaveValue('小爱同学一代')
    expect(nameInputs[1]).toHaveValue('Redmi小爱音箱Play')
    expect(nameInputs[2]).toHaveValue('小爱音箱Pro')
    // 地址：正常设备预填
    const addressInputs = screen.getAllByRole('textbox', { name: 'Address' })
    expect(addressInputs[0]).toHaveValue('192.168.31.142')
    expect(addressInputs[1]).toHaveValue('192.168.31.232')
    // 离线设备的 0.0.0.1 占位地址不预填，留给用户手填
    expect(addressInputs[2]).toHaveValue('')
  })

  it('creates one output per device, carrying the shared account fields', async () => {
    renderDialog()
    await pickXiaomiAndSimulateSelection()
    await goToStep3()

    // 填小米账号（所有音箱共用）
    fireEvent.change(screen.getByRole('textbox', { name: 'Xiaomi account' }), {
      target: { value: '1250258297' },
    })
    fireEvent.change(screen.getByLabelText(/passToken/i), {
      target: { value: 'V1:testtoken' },
    })

    // 给离线那台手填地址
    const addressInputs = screen.getAllByRole('textbox', { name: 'Address' })
    fireEvent.change(addressInputs[2], { target: { value: '192.168.31.200' } })
    expect(addressInputs[2]).toHaveValue('192.168.31.200')

    // 给第二台改个名和 ID
    const nameInputs = screen.getAllByLabelText('Name')
    fireEvent.change(nameInputs[1], { target: { value: '书房音箱' } })
    const idInputs = screen.getAllByRole('textbox', { name: 'ID' })
    fireEvent.change(idInputs[1], { target: { value: 'study' } })

    const saveButton = screen.getByRole('button', { name: 'Save' })
    fireEvent.click(saveButton)

    await waitFor(() => expect(postMock).toHaveBeenCalledTimes(3))

    const payloads = postMock.mock.calls.map(([, opts]) =>
      JSON.parse(opts.body),
    )
    expect(payloads).toHaveLength(3)
    // 全部是 xiaomi 类型，且都带上了共用的账号凭据
    for (const p of payloads) {
      expect(p.type).toBe('xiaomi')
      expect(p.account).toBe('1250258297')
      expect(p.passToken).toBe('V1:testtoken')
      // 中间状态不能混进 payload
      expect(p.xiaomiDevices).toBeUndefined()
    }
    // 修改生效
    const study = payloads.find((p) => p.id === 'study')
    expect(study).toBeTruthy()
    expect(study.name).toBe('书房音箱')
    expect(study.address).toBe('192.168.31.232')
    // 手填的地址被采纳
    expect(payloads.some((p) => p.address === '192.168.31.200')).toBe(true)
    // ID 唯一
    expect(new Set(payloads.map((p) => p.id)).size).toBe(3)

    await waitFor(() => expect(mockOnSaved).toHaveBeenCalled())
  })

  it('blocks saving while a placeholder 0.0.0.x address is left in place', async () => {
    renderDialog()
    await pickXiaomiAndSimulateSelection()
    await goToStep3()

    // 不给离线那台填地址
    const saveButton = screen.getByRole('button', { name: 'Save' })
    await waitFor(() => expect(saveButton).toBeDisabled())
    expect(postMock).not.toHaveBeenCalled()
  })
})
