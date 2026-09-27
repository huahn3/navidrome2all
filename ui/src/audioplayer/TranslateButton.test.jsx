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

// partial mock：组件会经 playerReducer 间接用到 actions 里的常量
vi.mock('../actions', async (importOriginal) => ({
  ...(await importOriginal()),
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
          bilingualTrackId: 'song1',
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

  it("does not treat another track's bilingual flag as this one being bilingual", () => {
    // 回归：双语态以前是全局 boolean。给 song1 开了双语后切到 song2，
    // song2 的按钮也会显示"已双语"，点一下却提示"已恢复原文歌词"——
    // 因为 song2 从来没被翻译过。
    useSelector.mockImplementation((selector) =>
      selector({
        player: {
          // song1 才是双语的
          bilingualTrackId: 'song1',
          current: { trackId: 'song2', lyric: 'Song2 Lyric' },
          originalLyrics: { song2: 'Song2 Original' },
          bilingualLyrics: { song1: 'Bilingual Lyric' },
        },
      }),
    )

    render(<TranslateButton id="song2" isRadio={false} isDesktop />)
    fireEvent.click(screen.getByTestId('translate-lyrics-button'))

    // song2 没有缓存译文，应该去请求翻译，而不是"恢复原文"
    expect(httpClient).toHaveBeenCalledWith(
      '/api/lyrics/translate',
      expect.objectContaining({ method: 'POST' }),
    )
    expect(updateSongLyric).not.toHaveBeenCalledWith(
      'song2',
      expect.anything(),
      false,
    )
  })

  it('does not block a different track while one translation is in flight', async () => {
    // 回归：在飞状态以前是全局 boolean。给 song1 翻译期间切到 song2，
    // song2 的按钮点不动；song1 请求结束后还会顺手把 song2 的 loading 状态清掉。
    useSelector.mockImplementation((selector) =>
      selector({ player: { current: { trackId: 'song1', lyric: 'L' } } }),
    )

    let resolveSong1
    let resolveSong2
    httpClient
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveSong1 = resolve
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveSong2 = resolve
          }),
      )

    const { rerender } = render(
      <TranslateButton id="song1" isRadio={false} isDesktop />,
    )
    fireEvent.click(screen.getByTestId('translate-lyrics-button'))
    expect(httpClient).toHaveBeenCalledTimes(1)

    // 切到 song2：必须能发起自己的请求，不能被 song1 挡住
    rerender(<TranslateButton id="song2" isRadio={false} isDesktop />)
    fireEvent.click(screen.getByTestId('translate-lyrics-button'))
    expect(httpClient).toHaveBeenCalledTimes(2)

    // song1 先结束：song2 还在飞，不能被顺手清掉 loading
    resolveSong1({ json: { inlineLrc: 'SONG1-BILINGUAL' } })
    await waitFor(() =>
      expect(updateSongLyric).toHaveBeenCalledWith(
        'song1',
        'SONG1-BILINGUAL',
        true,
      ),
    )
    // 关键断言：song2 必须仍在翻译中（转圈 + 提示语）
    expect(screen.getByRole('progressbar')).toBeTruthy()
    expect(
      screen.getByTestId('translate-lyrics-button').getAttribute('aria-label'),
    ).not.toBe('翻译歌词 (双语对照，右键单击强制重译)')

    // song2 自己的结果随后正常落地
    resolveSong2({ json: { inlineLrc: 'SONG2-BILINGUAL' } })
    await waitFor(() =>
      expect(updateSongLyric).toHaveBeenCalledWith(
        'song2',
        'SONG2-BILINGUAL',
        true,
      ),
    )
    expect(screen.queryByRole('progressbar')).toBeNull()
  })
})
