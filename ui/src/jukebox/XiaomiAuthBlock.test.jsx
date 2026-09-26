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
    expect(screen.getByText('选用此音箱')).toBeInTheDocument()

    // Click "选用此音箱"
    fireEvent.click(screen.getByText('选用此音箱'))

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
  it('adds several checked speakers as outputs in one go', async () => {
    const post = vi.fn().mockResolvedValue({ ok: true, json: async () => ({}) })
    const get = vi.fn().mockResolvedValue({ ok: true, json: async () => [] })
    vi.stubGlobal(
      'fetch',
      vi.fn((url, opts) =>
        opts?.method === 'POST' ? post(url, opts) : get(url, opts),
      ),
    )
    const onBatchCreated = vi.fn()

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
        onBatchCreated={onBatchCreated}
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

    // Nothing selected yet: the batch button stays disabled.
    expect(
      screen.getByRole('button', { name: '添加所选 0 台为输出设备' }),
    ).toBeDisabled()

    fireEvent.click(screen.getByLabelText('选择 小爱同学一代'))
    fireEvent.click(screen.getByLabelText('选择 Redmi小爱音箱Play'))
    expect(
      screen.getByRole('button', { name: '添加所选 2 台为输出设备' }),
    ).toBeEnabled()

    fireEvent.click(
      screen.getByRole('button', { name: '添加所选 2 台为输出设备' }),
    )

    await waitFor(() => expect(post).toHaveBeenCalledTimes(2))
    const bodies = post.mock.calls.map(([, opts]) => JSON.parse(opts.body))
    expect(bodies.map((b) => b.address)).toEqual([
      '192.168.31.142',
      '192.168.31.232',
    ])
    expect(bodies[0]).toMatchObject({
      type: 'xiaomi',
      did: 'd1',
      model: 's12',
      account: '1250258297',
    })
    // Names are Chinese, so the id falls back to the <type>-<hash> form and must stay unique
    expect(new Set(bodies.map((b) => b.id)).size).toBe(2)
    await waitFor(() => expect(onBatchCreated).toHaveBeenCalled())
    expect(onBatchCreated.mock.calls[0][0].created).toHaveLength(2)
    expect(screen.getByText(/已添加 2 台输出设备/)).toBeInTheDocument()
  })

  it('reports per-device failures and keeps the dialog data', async () => {
    const post = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({}) })
      .mockResolvedValueOnce({
        ok: false,
        text: async () => 'an output with this id already exists',
      })
    const get = vi.fn().mockResolvedValue({ ok: true, json: async () => [] })
    vi.stubGlobal(
      'fetch',
      vi.fn((url, opts) =>
        opts?.method === 'POST' ? post(url, opts) : get(url, opts),
      ),
    )
    const onBatchCreated = vi.fn()

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
        {
          did: 'd2',
          name: '音箱乙',
          model: 'xiaomi.wifispeaker.l7a',
          localip: '192.168.31.2',
          token: 'tk2',
        },
      ],
    })

    render(
      <XiaomiAuthBlock
        formData={{ type: 'xiaomi' }}
        isCreate={true}
        onBatchCreated={onBatchCreated}
      />,
    )
    fireEvent.click(screen.getByText('passToken 凭证直填'))
    const inputs = screen.getAllByRole('textbox')
    fireEvent.change(inputs[0], { target: { value: '1' } })
    fireEvent.change(inputs[1], { target: { value: 'V1:t' } })
    fireEvent.click(screen.getByText('校验凭证并读取设备列表'))
    await waitFor(() => expect(screen.getByText('音箱甲')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: '全选' }))
    fireEvent.click(
      screen.getByRole('button', { name: '添加所选 2 台为输出设备' }),
    )

    await waitFor(() => expect(onBatchCreated).toHaveBeenCalled())
    const { created, failed } = onBatchCreated.mock.calls[0][0]
    expect(created).toHaveLength(1)
    expect(failed).toHaveLength(1)
    expect(screen.getByText(/1 台失败/)).toBeInTheDocument()
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
