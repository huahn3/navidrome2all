import React from 'react'
import { render, screen, fireEvent, cleanup } from '@testing-library/react'
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { useDispatch, useSelector } from 'react-redux'
import VolumeControl from './VolumeControl'
import { setVolume } from '../actions'

const mockDispatch = vi.fn()

vi.mock('react-redux', () => ({
  useDispatch: vi.fn(),
  useSelector: vi.fn(),
}))

vi.mock('react-admin', () => ({
  useTranslate: () => (key) => key,
}))

const renderWithVolume = (volume) => {
  useSelector.mockImplementation((selector) => selector({ player: { volume } }))
  return render(<VolumeControl />)
}

describe('<VolumeControl />', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useDispatch.mockReturnValue(mockDispatch)
  })

  afterEach(cleanup)

  it('shows the stored volume as a percentage', () => {
    renderWithVolume(0.6)
    expect(screen.getByTestId('volume-control')).toHaveTextContent('60%')
  })

  it('falls back to full volume when the store has no value', () => {
    renderWithVolume(undefined)
    expect(screen.getByTestId('volume-control')).toHaveTextContent('100%')
  })

  it('dispatches the new volume when the slider moves', () => {
    renderWithVolume(0.6)

    fireEvent.keyDown(screen.getByRole('slider'), { key: 'ArrowRight' })

    expect(mockDispatch).toHaveBeenCalledWith(setVolume(0.61))
  })

  it('mutes and restores the previous level', () => {
    const { rerender } = renderWithVolume(0.6)

    fireEvent.click(screen.getByRole('button'))
    expect(mockDispatch).toHaveBeenLastCalledWith(setVolume(0))

    useSelector.mockImplementation((selector) =>
      selector({ player: { volume: 0 } }),
    )
    rerender(<VolumeControl />)
    fireEvent.click(screen.getByRole('button'))

    expect(mockDispatch).toHaveBeenLastCalledWith(setVolume(0.6))
  })
})
