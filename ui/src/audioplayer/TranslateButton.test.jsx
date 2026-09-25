import React from 'react'
import {
  render,
  screen,
  fireEvent,
  cleanup,
  waitFor,
} from '@testing-library/react'
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { useDispatch, useSelector } from 'react-redux'
import TranslateButton from './TranslateButton'
import httpClient from '../dataProvider/httpClient'
import { updateSongLyric } from '../actions'

const mockDispatch = vi.fn()
const mockNotify = vi.fn()

vi.mock('react-redux', () => ({
  useDispatch: vi.fn(),
  useSelector: vi.fn(),
}))

vi.mock('react-admin', () => ({
  useTranslate: () => (key, options) => options?._ || key,
  useNotify: () => mockNotify,
}))

vi.mock('../dataProvider/httpClient', () => ({
  default: vi.fn(),
}))

vi.mock('../actions', () => ({
  updateSongLyric: vi.fn((trackId, lyric, isBilingual) => ({
    type: 'PLAYER_UPDATE_SONG_LYRIC',
    data: { trackId, lyric, isBilingual },
  })),
}))

describe('<TranslateButton />', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useDispatch.mockReturnValue(mockDispatch)
  })

  afterEach(cleanup)

  it('renders disabled when isRadio or no id', () => {
    useSelector.mockImplementation((selector) =>
      selector({ player: { bilingualActive: false } }),
    )
    const { rerender } = render(
      <TranslateButton id="" isRadio={false} isDesktop />,
    )
    expect(screen.getByTestId('translate-lyrics-button')).toBeDisabled()

    rerender(<TranslateButton id="song1" isRadio={true} isDesktop />)
    expect(screen.getByTestId('translate-lyrics-button')).toBeDisabled()
  })

  it('toggles back to original lyrics when bilingual is active', () => {
    useSelector.mockImplementation((selector) =>
      selector({
        player: {
          bilingualActive: true,
          current: { trackId: 'song1', lyric: 'Bilingual Lyric' },
          originalLyrics: { song1: 'Original Lyric' },
          bilingualLyrics: { song1: 'Bilingual Lyric' },
        },
      }),
    )

    render(<TranslateButton id="song1" isRadio={false} isDesktop />)
    const button = screen.getByTestId('translate-lyrics-button')
    fireEvent.click(button)

    expect(updateSongLyric).toHaveBeenCalledWith(
      'song1',
      'Original Lyric',
      false,
    )
    expect(mockDispatch).toHaveBeenCalled()
    expect(mockNotify).toHaveBeenCalledWith('已恢复原文歌词', expect.anything())
  })

  it('switches to cached bilingual lyrics without API call if already cached', () => {
    useSelector.mockImplementation((selector) =>
      selector({
        player: {
          bilingualActive: false,
          current: { trackId: 'song1', lyric: 'Original Lyric' },
          originalLyrics: { song1: 'Original Lyric' },
          bilingualLyrics: { song1: 'Cached Bilingual Lyric' },
        },
      }),
    )

    render(<TranslateButton id="song1" isRadio={false} isDesktop />)
    const button = screen.getByTestId('translate-lyrics-button')
    fireEvent.click(button)

    expect(updateSongLyric).toHaveBeenCalledWith(
      'song1',
      'Cached Bilingual Lyric',
      true,
    )
    expect(mockDispatch).toHaveBeenCalled()
    expect(httpClient).not.toHaveBeenCalled()
    expect(mockNotify).toHaveBeenCalledWith(
      '已切换为双语对照歌词',
      expect.anything(),
    )
  })

  it('fetches translation from backend when not yet cached', async () => {
    useSelector.mockImplementation((selector) =>
      selector({
        player: {
          bilingualActive: false,
          current: { trackId: 'song1', lyric: 'Original Lyric' },
          originalLyrics: {},
          bilingualLyrics: {},
        },
      }),
    )

    httpClient.mockResolvedValueOnce({
      json: {
        inlineLrc: '[00:01.00] 原文 / Translation',
      },
    })

    render(<TranslateButton id="song1" isRadio={false} isDesktop />)
    const button = screen.getByTestId('translate-lyrics-button')
    fireEvent.click(button)

    expect(httpClient).toHaveBeenCalledWith(
      '/api/lyrics/translate',
      expect.anything(),
    )
    await waitFor(() => {
      expect(updateSongLyric).toHaveBeenCalledWith(
        'song1',
        '[00:01.00] 原文 / Translation',
        true,
      )
    })
    expect(mockNotify).toHaveBeenCalledWith(
      '歌词翻译完成，已显示双语对照',
      expect.anything(),
    )
  })

  it('forces re-translation on right click (contextmenu) with force: true', async () => {
    useSelector.mockImplementation((selector) =>
      selector({
        player: {
          bilingualActive: true,
          originalLyrics: { song1: '[00:01.00] 原文' },
          bilingualLyrics: { song1: '[00:01.00] 旧翻译' },
        },
      }),
    )

    httpClient.mockResolvedValueOnce({
      json: {
        inlineLrc: '[00:01.00] 原文 / 新模型翻译',
      },
    })

    render(<TranslateButton id="song1" isRadio={false} isDesktop />)
    const button = screen.getByTestId('translate-lyrics-button')
    fireEvent.contextMenu(button)

    expect(httpClient).toHaveBeenCalledWith(
      '/api/lyrics/translate',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          songId: 'song1',
          force: true,
        }),
      }),
    )

    await waitFor(() => {
      expect(updateSongLyric).toHaveBeenCalledWith(
        'song1',
        '[00:01.00] 原文 / 新模型翻译',
        true,
      )
    })
  })
})
