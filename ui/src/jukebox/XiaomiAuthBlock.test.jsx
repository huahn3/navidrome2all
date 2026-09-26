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
})
