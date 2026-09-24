import React from 'react'
import { render, screen, fireEvent, cleanup } from '@testing-library/react'
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { useDispatch, useSelector } from 'react-redux'
import DeviceSelector from './DeviceSelector'
import config from '../config'
import * as jukebox from './jukebox'

const mockDispatch = vi.fn()

vi.mock('react-redux', () => ({
  useDispatch: vi.fn(),
  useSelector: vi.fn(),
}))

vi.mock('react-admin', () => ({
  useTranslate: () => (key) => key,
}))

vi.mock('../actions', () => ({
  BROWSER_DEVICE: 'browser',
  setOutputDevice: vi.fn((deviceId) => ({
    type: 'PLAYER_SET_OUTPUT_DEVICE',
    data: { deviceId },
  })),
}))

vi.mock('./jukebox', () => ({
  __esModule: true,
  fetchDevices: vi.fn(),
}))

describe('<DeviceSelector />', () => {
  const devices = [
    { id: 'browser', name: 'Browser', type: 'builtin' },
    { id: 'xiaoai', name: 'Xiaoai speaker', type: 'dlna' },
  ]

  beforeEach(() => {
    vi.clearAllMocks()
    config.jukeboxEnabled = true
    useDispatch.mockReturnValue(mockDispatch)
    useSelector.mockImplementation((selector) =>
      selector({ player: { outputDevice: 'browser' } }),
    )
    jukebox.fetchDevices.mockResolvedValue({ devices, selected: 'browser' })
  })

  afterEach(cleanup)

  it('renders nothing when the jukebox is disabled', () => {
    config.jukeboxEnabled = false
    const { container } = render(<DeviceSelector />)
    expect(container).toBeEmptyDOMElement()
  })

  it('lists the available outputs and marks the selected one', async () => {
    render(<DeviceSelector />)
    fireEvent.click(screen.getByTestId('device-selector-button'))

    expect(await screen.findByText('jukebox.browser')).toBeInTheDocument()
    expect(await screen.findByText('Xiaoai speaker')).toBeInTheDocument()
    expect(jukebox.fetchDevices).toHaveBeenCalled()
  })

  it('dispatches the selection when picking another output', async () => {
    render(<DeviceSelector />)
    fireEvent.click(screen.getByTestId('device-selector-button'))

    fireEvent.click(await screen.findByText('Xiaoai speaker'))

    expect(mockDispatch).toHaveBeenCalledWith({
      type: 'PLAYER_SET_OUTPUT_DEVICE',
      data: { deviceId: 'xiaoai' },
    })
  })

  it('does not dispatch when re-selecting the current output', async () => {
    render(<DeviceSelector />)
    fireEvent.click(screen.getByTestId('device-selector-button'))

    fireEvent.click(await screen.findByText('jukebox.browser'))

    expect(mockDispatch).not.toHaveBeenCalled()
  })
})
