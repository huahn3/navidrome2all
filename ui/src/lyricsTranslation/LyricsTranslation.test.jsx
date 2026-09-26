import React from 'react'
import {
  render,
  screen,
  fireEvent,
  cleanup,
  waitFor,
} from '@testing-library/react'
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import LyricsTranslation from './LyricsTranslation'
import httpClient from '../dataProvider/httpClient'

const mockNotify = vi.fn()
const mockTranslate = (key, options) => options?._ || key

vi.mock('react-admin', () => ({
  useTranslate: () => mockTranslate,
  useNotify: () => mockNotify,
  Title: ({ title }) => <div data-testid="page-title">{title}</div>,
}))

vi.mock('../dataProvider/httpClient', () => ({
  default: vi.fn(),
}))

describe('<LyricsTranslation />', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    httpClient.mockImplementation((url, options) => {
      if (url === '/api/lyrics/translation/config') {
        return Promise.resolve({
          json: {
            enabled: true,
            engine: 'gemini',
            model: 'gemini-flash-latest',
            apiKey: '******',
            targetLanguage: 'zh-CN',
          },
        })
      }
      if (url === '/api/lyrics/translation/cache') {
        return Promise.resolve({
          json: {
            items: [
              {
                songId: 'song_1',
                title: '测试歌曲 1',
                artist: '歌手 A',
                targetLang: 'zh-CN',
                engine: 'gemini',
                model: 'gemini-flash-latest',
                updatedAt: '2026-09-26T00:00:00Z',
              },
            ],
            total: 1,
          },
        })
      }
      if (url === '/api/lyrics/translation/retranslate-status') {
        return Promise.resolve({
          json: {
            running: false,
            total: 0,
            processed: 0,
            success: 0,
            failed: 0,
          },
        })
      }
      return Promise.resolve({ json: { status: 'ok' } })
    })
  })

  afterEach(cleanup)

  it('loads and renders translation configuration and cached songs list', async () => {
    render(<LyricsTranslation />)

    await waitFor(() => {
      expect(screen.getByText('歌词双语翻译')).toBeInTheDocument()
    })

    expect(screen.getByText('已翻译歌曲管理与重新翻译')).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByText('测试歌曲 1')).toBeInTheDocument()
    })
  })

  it('submits updated config when Save button is clicked', async () => {
    render(<LyricsTranslation />)

    await waitFor(() => {
      expect(screen.getByText('歌词双语翻译')).toBeInTheDocument()
    })

    const saveBtn = screen.getByText('保存配置')
    fireEvent.click(saveBtn)

    await waitFor(() => {
      expect(httpClient).toHaveBeenCalledWith(
        '/api/lyrics/translation/config',
        expect.objectContaining({
          method: 'PUT',
        }),
      )
    })
  })

  it('opens confirmation dialog when clicking 重新翻译所有歌曲 and calls retranslate-all on confirm', async () => {
    render(<LyricsTranslation />)

    await waitFor(() => {
      expect(screen.getByText('测试歌曲 1')).toBeInTheDocument()
    })

    const retranslateAllBtn = screen.getByText('重新翻译所有歌曲')
    fireEvent.click(retranslateAllBtn)

    // Warning dialog should appear
    expect(screen.getByText('警告：确认重新翻译所有歌曲？')).toBeInTheDocument()
    expect(screen.getByText(/确定要立即重新翻译所有歌曲吗/)).toBeInTheDocument()

    // Confirm button in dialog
    const confirmBtn = screen.getByText('确认重新翻译全部歌曲')
    fireEvent.click(confirmBtn)

    await waitFor(() => {
      expect(httpClient).toHaveBeenCalledWith(
        '/api/lyrics/translation/retranslate-all',
        expect.objectContaining({
          method: 'POST',
        }),
      )
    })
  })

  it('calls single song re-translation when clicking 重新翻译 on a song item', async () => {
    render(<LyricsTranslation />)

    await waitFor(() => {
      expect(screen.getByText('测试歌曲 1')).toBeInTheDocument()
    })

    const retranslateTooltip =
      screen.getByTitle('使用当前选定模型重新翻译此歌曲')
    const retranslateBtn =
      retranslateTooltip.querySelector('button') || retranslateTooltip
    fireEvent.click(retranslateBtn)

    await waitFor(() => {
      expect(httpClient).toHaveBeenCalledWith(
        '/api/lyrics/translate',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            songId: 'song_1',
            targetLang: 'zh-CN',
            force: true,
          }),
        }),
      )
    })
  })
})
