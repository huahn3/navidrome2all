import React from 'react'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import XiaomiAuthBlock from './XiaomiAuthBlock'
import * as jukeboxApi from '../audioplayer/jukebox'

const mockChange = vi.fn()

vi.mock('react-final-form', () => ({
  useForm: () => ({
    change: mockChange,
  }),
}))

describe('<XiaomiAuthBlock />', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the 3 authentication option tabs', () => {
    render(<XiaomiAuthBlock formData={{ type: 'xiaomi' }} isCreate={true} />)

    expect(screen.getByText('米家扫码登录 (推荐)')).toBeInTheDocument()
    expect(screen.getByText('账号密码登录')).toBeInTheDocument()
    expect(screen.getByText('passToken 凭证直填')).toBeInTheDocument()
  })

  it('switches between tabs correctly', () => {
    render(<XiaomiAuthBlock formData={{ type: 'xiaomi' }} isCreate={true} />)

    // Initially on Tab 0: QR Scan
    expect(screen.getByText('生成登录二维码')).toBeInTheDocument()

    // Switch to Tab 1: Password
    fireEvent.click(screen.getByText('账号密码登录'))
    expect(screen.getByText('登录并读取设备列表')).toBeInTheDocument()

    // Switch to Tab 2: PassToken
    fireEvent.click(screen.getByText('passToken 凭证直填'))
    expect(screen.getByText('校验凭证并读取设备列表')).toBeInTheDocument()
    expect(screen.getByText(/MI-PassToken-Helper/)).toBeInTheDocument()
  })

  it('handles passToken login and one-click autofills the chosen speaker', async () => {
    vi.spyOn(jukeboxApi, 'loginXiaomiPassToken').mockResolvedValueOnce({
      status: 'success',
      userId: '1250258297',
      passToken: 'V1:testtoken',
      devices: [
        {
          did: '337446413',
          name: 'Redmi小爱音箱Play',
          model: 'xiaomi.wifispeaker.l7a',
          localip: '192.168.31.232',
          token: '4a67646846584c336d4353356d577942',
          isOnline: true,
        },
      ],
    })

    render(<XiaomiAuthBlock formData={{ type: 'xiaomi' }} isCreate={true} />)

    // Switch to passToken tab
    fireEvent.click(screen.getByText('passToken 凭证直填'))

    // Fill in inputs
    const inputs = screen.getAllByRole('textbox')
    fireEvent.change(inputs[0], { target: { value: '1250258297' } })
    fireEvent.change(inputs[1], { target: { value: 'V1:testtoken' } })

    // Click submit
    fireEvent.click(screen.getByText('校验凭证并读取设备列表'))

    // Should display speaker device in the success list
    await waitFor(() => {
      expect(screen.getByText('Redmi小爱音箱Play')).toBeInTheDocument()
    })
    // 勾选即预填：不再有单独的「选用此音箱」按钮
    expect(screen.queryByText('选用此音箱')).not.toBeInTheDocument()
    fireEvent.click(screen.getByLabelText('选择 Redmi小爱音箱Play'))

    // Verify form.change was called for speaker configuration
    expect(mockChange).toHaveBeenCalledWith('name', 'Redmi小爱音箱Play')
    expect(mockChange).toHaveBeenCalledWith('address', '192.168.31.232')
    expect(mockChange).toHaveBeenCalledWith(
      'token',
      '4a67646846584c336d4353356d577942',
    )
    expect(mockChange).toHaveBeenCalledWith('did', '337446413')
    expect(mockChange).toHaveBeenCalledWith('model', 'l7a')
    expect(mockChange).toHaveBeenCalledWith('account', '1250258297')
    expect(mockChange).toHaveBeenCalledWith('passToken', 'V1:testtoken')
    expect(mockChange).toHaveBeenCalledWith('id', 'xiaomi_l7a')
  })
  it('keeps multi-select working and reports every checked device', async () => {
    // 勾选不再直接创建输出设备：职责交给向导的第 3 步（逐台填表后统一提交）
    const onSelectionChange = vi.fn()
    vi.spyOn(jukeboxApi, 'loginXiaomiPassToken').mockResolvedValueOnce({
      status: 'success',
      userId: '1250258297',
      passToken: 'V1:testtoken',
      devices: [
        {
          did: 'd1',
          name: '小爱同学一代',
          model: 'xiaomi.wifispeaker.s12',
          localip: '192.168.31.142',
          token: 'tk1',
          isOnline: true,
        },
        {
          did: 'd2',
          name: 'Redmi小爱音箱Play',
          model: 'xiaomi.wifispeaker.l7a',
          localip: '192.168.31.232',
          token: 'tk2',
          isOnline: true,
        },
      ],
    })

    render(
      <XiaomiAuthBlock
        formData={{ type: 'xiaomi' }}
        isCreate={true}
        onSelectionChange={onSelectionChange}
      />,
    )

    fireEvent.click(screen.getByText('passToken 凭证直填'))
    const inputs = screen.getAllByRole('textbox')
    fireEvent.change(inputs[0], { target: { value: '1250258297' } })
    fireEvent.change(inputs[1], { target: { value: 'V1:testtoken' } })
    fireEvent.click(screen.getByText('校验凭证并读取设备列表'))

    await waitFor(() =>
      expect(screen.getByText('小爱同学一代')).toBeInTheDocument(),
    )

    // 批量创建按钮已经没有了：勾选只负责选择，创建由向导提交
    expect(
      screen.queryByRole('button', { name: /添加所选 \d+ 台为输出设备/ }),
    ).not.toBeInTheDocument()

    fireEvent.click(screen.getByLabelText('选择 小爱同学一代'))
    fireEvent.click(screen.getByLabelText('选择 Redmi小爱音箱Play'))

    // 两台都在，且都已勾上（"只能勾选一个"是这个 bug）
    expect(screen.getByLabelText('选择 小爱同学一代')).toBeChecked()
    expect(screen.getByLabelText('选择 Redmi小爱音箱Play')).toBeChecked()
    expect(screen.getByText('已选 2 台')).toBeInTheDocument()

    // 父组件拿到了两台，第 3 步据此渲染两份表单
    await waitFor(() => {
      const last = onSelectionChange.mock.calls.at(-1)?.[0] || []
      expect(last.map((d) => d.did)).toEqual(['d1', 'd2'])
    })
  })

  it('lets offline speakers be checked but flags their placeholder IP', async () => {
    // 离线设备的小米云会回 0.0.0.1。以前直接禁掉勾选框，结果"只能勾选一台"；
    // 现在允许勾选（用户自己知道真实 IP），但列表标注"需手填"且不预填地址。
    const mockChange2 = vi.fn()
    vi.doMock('react-final-form', async () => ({
      ...(await vi.importActual('react-final-form')),
      useForm: () => ({ change: mockChange2 }),
    }))
    vi.spyOn(jukeboxApi, 'loginXiaomiPassToken').mockResolvedValueOnce({
      status: 'success',
      userId: '1',
      passToken: 'V1:t',
      devices: [
        {
          did: 'd1',
          name: '离线音箱',
          model: 'xiaomi.wifispeaker.l7a',
          localip: '0.0.0.1',
          isOnline: false,
          token: 'tk1',
        },
      ],
    })

    render(<XiaomiAuthBlock formData={{ type: 'xiaomi' }} isCreate={true} />)
    fireEvent.click(screen.getByText('passToken 凭证直填'))
    const inputs = screen.getAllByRole('textbox')
    fireEvent.change(inputs[0], { target: { value: '1' } })
    fireEvent.change(inputs[1], { target: { value: 'V1:t' } })
    fireEvent.click(screen.getByText('校验凭证并读取设备列表'))

    await waitFor(() =>
      expect(screen.getByText('离线音箱')).toBeInTheDocument(),
    )
    // 可以勾选了
    expect(screen.getByLabelText('选择 离线音箱')).toBeEnabled()
    // 并且明确标出来源
    expect(screen.getByText('局域网 IP: 未知，需手填')).toBeInTheDocument()
  })

  it('never creates an output on its own: no bare fetch, no create call', async () => {
    // 回归：批量创建曾用裸 fetch（没有 X-ND-Authorization），后端一律 401
    // Not authenticated，表现为"已添加 0 台，N 台失败"。
    // 现在这个块只负责**选择**，创建一律由向导第 3 步提交，所以这里断言
    // 勾选过程中一次请求都不该发出。
    const bareFetch = vi.fn(() => {
      throw new Error('raw fetch must not be used for the jukebox outputs API')
    })
    vi.stubGlobal('fetch', bareFetch)
    const createJukeboxOutput = vi.spyOn(jukeboxApi, 'createJukeboxOutput')
    const listJukeboxOutputs = vi.spyOn(jukeboxApi, 'listJukeboxOutputs')

    vi.spyOn(jukeboxApi, 'loginXiaomiPassToken').mockResolvedValueOnce({
      status: 'success',
      userId: '1',
      passToken: 'V1:t',
      devices: [
        {
          did: 'd1',
          name: '音箱甲',
          model: 'xiaomi.wifispeaker.s12',
          localip: '192.168.31.1',
          token: 'tk1',
        },
      ],
    })

    render(<XiaomiAuthBlock formData={{ type: 'xiaomi' }} isCreate={true} />)
    fireEvent.click(screen.getByText('passToken 凭证直填'))
    const inputs = screen.getAllByRole('textbox')
    fireEvent.change(inputs[0], { target: { value: '1' } })
    fireEvent.change(inputs[1], { target: { value: 'V1:t' } })
    fireEvent.click(screen.getByText('校验凭证并读取设备列表'))
    await waitFor(() => expect(screen.getByText('音箱甲')).toBeInTheDocument())

    fireEvent.click(screen.getByLabelText('选择 音箱甲'))
    await waitFor(() =>
      expect(screen.getByText('已选 1 台')).toBeInTheDocument(),
    )

    expect(bareFetch).not.toHaveBeenCalled()
    expect(createJukeboxOutput).not.toHaveBeenCalled()
    expect(listJukeboxOutputs).not.toHaveBeenCalled()
  })

  it('prefills the form as soon as a speaker is checked', async () => {
    // 回归：以前要点「选用此音箱」才填表，勾选本身没反应，
    // 于是勾完直接点"下一步"看到的是空表单
    vi.spyOn(jukeboxApi, 'loginXiaomiPassToken').mockResolvedValueOnce({
      status: 'success',
      userId: '1250258297',
      passToken: 'V1:testtoken',
      devices: [
        {
          did: 'd1',
          name: '音箱甲',
          model: 'xiaomi.wifispeaker.s12',
          localip: '192.168.31.142',
          token: 'tk1',
        },
        {
          did: 'd2',
          name: '音箱乙',
          model: 'xiaomi.wifispeaker.l7a',
          localip: '192.168.31.232',
          token: 'tk2',
        },
      ],
    })

    render(<XiaomiAuthBlock formData={{ type: 'xiaomi' }} isCreate={true} />)
    fireEvent.click(screen.getByText('passToken 凭证直填'))
    const inputs = screen.getAllByRole('textbox')
    fireEvent.change(inputs[0], { target: { value: '1' } })
    fireEvent.change(inputs[1], { target: { value: 'V1:t' } })
    fireEvent.click(screen.getByText('校验凭证并读取设备列表'))
    await waitFor(() => expect(screen.getByText('音箱甲')).toBeInTheDocument())

    mockChange.mockClear()
    fireEvent.click(screen.getByLabelText('选择 音箱甲'))
    expect(mockChange).toHaveBeenCalledWith('name', '音箱甲')
    expect(mockChange).toHaveBeenCalledWith('address', '192.168.31.142')
    expect(mockChange).toHaveBeenCalledWith('did', 'd1')

    // 取消勾选后改用剩下的第一台预填，表单不留在已取消的设备上
    mockChange.mockClear()
    fireEvent.click(screen.getByLabelText('选择 音箱甲'))
    fireEvent.click(screen.getByLabelText('选择 音箱乙'))
    expect(mockChange).toHaveBeenCalledWith('name', '音箱乙')
    expect(mockChange).toHaveBeenCalledWith('address', '192.168.31.232')
  })

  it('does not offer devices without a local IP', async () => {
    vi.spyOn(jukeboxApi, 'loginXiaomiPassToken').mockResolvedValueOnce({
      status: 'success',
      userId: '1',
      passToken: 'V1:t',
      devices: [
        {
          did: 'd1',
          name: '离线音箱',
          model: 'xiaomi.wifispeaker.s12',
          localip: '',
          token: 'tk1',
        },
      ],
    })

    render(<XiaomiAuthBlock formData={{ type: 'xiaomi' }} isCreate={true} />)
    fireEvent.click(screen.getByText('passToken 凭证直填'))
    const inputs = screen.getAllByRole('textbox')
    fireEvent.change(inputs[0], { target: { value: '1' } })
    fireEvent.change(inputs[1], { target: { value: 'V1:t' } })
    fireEvent.click(screen.getByText('校验凭证并读取设备列表'))

    await waitFor(() =>
      expect(screen.getByText('离线音箱')).toBeInTheDocument(),
    )
    expect(screen.getByLabelText('选择 离线音箱')).toBeDisabled()
  })
})
