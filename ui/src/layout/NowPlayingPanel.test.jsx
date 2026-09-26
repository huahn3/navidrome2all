import React from 'react'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, beforeEach, vi } from 'vitest'
import { Provider } from 'react-redux'
import { createStore, combineReducers } from 'redux'
import { activityReducer } from '../reducers'
import NowPlayingPanel from './NowPlayingPanel'
import { formatDeviceName } from '../utils'
import subsonic from '../subsonic'
import httpClient from '../dataProvider/httpClient'

vi.mock('../subsonic', () => ({
  default: {
    getNowPlaying: vi.fn(),
    getAvatarUrl: vi.fn(() => '/avatar'),
    getCoverArtUrl: vi.fn(() => '/cover'),
    getSong: vi.fn().mockResolvedValue({
      json: {
        'subsonic-response': {
          status: 'ok',
          song: { id: 'song1', title: 'Song', artist: 'Artist' },
        },
      },
    }),
  },
}))

vi.mock('../dataProvider/httpClient', () => ({
  default: vi.fn().mockResolvedValue({ json: { status: 'ok' } }),
  clientUniqueId: 'test-client-unique-id',
}))

// Create a mock for useMediaQuery
const mockUseMediaQuery = vi.fn()

vi.mock('react-admin', async (importOriginal) => {
  const actual = await importOriginal()
  const redux = await import('react-redux')
  return {
    ...actual,
    useTranslate: () => (x) => x,
    useSelector: redux.useSelector,
    useDispatch: redux.useDispatch,
    Link: ({ to, children, onClick, ...props }) => (
      <a
        href={to}
        onClick={(e) => {
          e.preventDefault() // Prevent navigation in tests
          if (onClick) onClick(e)
        }}
        {...props}
      >
        {children}
      </a>
    ),
  }
})

// Mock the specific Material-UI hooks we need
vi.mock('@material-ui/core/useMediaQuery', () => ({
  default: () => mockUseMediaQuery(),
}))

vi.mock('@material-ui/core/styles/useTheme', () => ({
  default: () => ({
    breakpoints: {
      down: () => '(max-width:959.95px)', // Mock breakpoint string
    },
  }),
}))

describe('<NowPlayingPanel />', () => {
  const createMockStore = (overrides = {}) => {
    const defaultState = {
      activity: {
        nowPlayingCount: 1,
        serverStart: { startTime: Date.now() }, // Server is up by default
        streamReconnected: 0,
        ...overrides,
      },
    }
    return createStore(
      combineReducers({ activity: activityReducer }),
      defaultState,
    )
  }

  afterEach(() => {
    vi.useRealTimers()
  })

  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    mockUseMediaQuery.mockReturnValue(false) // Default to large screen

    subsonic.getNowPlaying.mockResolvedValue({
      json: {
        'subsonic-response': {
          status: 'ok',
          nowPlaying: {
            entry: [
              {
                playerId: 1,
                username: 'u1',
                playerName: 'Chrome Browser',
                title: 'Song',
                albumArtist: 'Artist',
                albumId: 'album1',
                albumArtistId: 'artist1',
                minutesAgo: 2,
              },
            ],
          },
        },
      },
    })
  })

  it('fetches and displays entries when opened', async () => {
    const store = createMockStore()
    render(
      <Provider store={store}>
        <NowPlayingPanel />
      </Provider>,
    )

    // Advance past debounce and flush promises
    await vi.advanceTimersByTimeAsync(500)

    fireEvent.click(screen.getByRole('button'))
    await waitFor(() => {
      expect(screen.getByText('Artist')).toBeInTheDocument()
      expect(screen.getByRole('link', { name: 'Artist' })).toHaveAttribute(
        'href',
        '/artist/artist1/show',
      )
    })
  })

  it('displays player name after username', async () => {
    const store = createMockStore()
    render(
      <Provider store={store}>
        <NowPlayingPanel />
      </Provider>,
    )

    await vi.advanceTimersByTimeAsync(500)

    fireEvent.click(screen.getByRole('button'))
    await waitFor(() => {
      expect(screen.getByText('u1 (Chrome Browser)')).toBeInTheDocument()
    })
  })

  it('handles entries without player name', async () => {
    subsonic.getNowPlaying.mockResolvedValue({
      json: {
        'subsonic-response': {
          status: 'ok',
          nowPlaying: {
            entry: [
              {
                playerId: 1,
                username: 'u1',
                title: 'Song',
                albumArtist: 'Artist',
                albumId: 'album1',
                albumArtistId: 'artist1',
                minutesAgo: 2,
              },
            ],
          },
        },
      },
    })

    const store = createMockStore()
    render(
      <Provider store={store}>
        <NowPlayingPanel />
      </Provider>,
    )

    await vi.advanceTimersByTimeAsync(500)

    fireEvent.click(screen.getByRole('button'))
    await waitFor(() => {
      expect(screen.getByText('u1')).toBeInTheDocument()
    })
  })

  it('shows empty message when no entries', async () => {
    subsonic.getNowPlaying.mockResolvedValue({
      json: {
        'subsonic-response': { status: 'ok', nowPlaying: { entry: [] } },
      },
    })
    const store = createMockStore({ nowPlayingCount: 0 })
    render(
      <Provider store={store}>
        <NowPlayingPanel />
      </Provider>,
    )

    await vi.advanceTimersByTimeAsync(500)

    fireEvent.click(screen.getByRole('button'))
    await waitFor(() => {
      expect(screen.getByText('nowPlaying.empty')).toBeInTheDocument()
    })
  })

  it('does not close panel when artist link is clicked on large screens', async () => {
    mockUseMediaQuery.mockReturnValue(false) // Simulate large screen

    const store = createMockStore()
    render(
      <Provider store={store}>
        <NowPlayingPanel />
      </Provider>,
    )

    await vi.advanceTimersByTimeAsync(500)

    // Open the panel
    fireEvent.click(screen.getByRole('button'))
    await waitFor(() => {
      expect(screen.getByText('Artist')).toBeInTheDocument()
    })

    // Check that the popover is open
    expect(screen.getByRole('presentation')).toBeInTheDocument()

    // Click the artist link
    fireEvent.click(screen.getByRole('link', { name: 'Artist' }))

    // Panel should remain open (popover should still be in document)
    expect(screen.getByRole('presentation')).toBeInTheDocument()
    expect(screen.getByText('Artist')).toBeInTheDocument()
  })

  it('does not fetch on mount when server is down', () => {
    const store = createMockStore({
      nowPlayingCount: 1,
      serverStart: { startTime: null }, // Server is down
    })
    render(
      <Provider store={store}>
        <NowPlayingPanel />
      </Provider>,
    )

    // Should not have made initial fetch request due to server being down
    expect(subsonic.getNowPlaying).not.toHaveBeenCalled()
  })

  it('does not fetch on stream reconnection when server is down', () => {
    const store = createMockStore({
      nowPlayingCount: 1,
      serverStart: { startTime: null }, // Server is down
      streamReconnected: Date.now(), // Stream reconnected
    })
    render(
      <Provider store={store}>
        <NowPlayingPanel />
      </Provider>,
    )

    // Should not have made fetch request due to server being down
    expect(subsonic.getNowPlaying).not.toHaveBeenCalled()
  })

  it('does not double-fetch on server reconnection', async () => {
    vi.useFakeTimers()

    const initialStore = createMockStore({
      nowPlayingCount: 1,
      serverStart: { startTime: null }, // Server initially down
      streamReconnected: 0,
    })
    const { rerender } = render(
      <Provider store={initialStore}>
        <NowPlayingPanel />
      </Provider>,
    )

    // Clear initial (empty) calls
    vi.clearAllMocks()

    // Simulate server coming back up with stream reconnection (both state changes happen)
    const reconnectedStore = createMockStore({
      nowPlayingCount: 1,
      serverStart: { startTime: Date.now() }, // Server back up
      streamReconnected: Date.now(), // Stream reconnected
    })
    rerender(
      <Provider store={reconnectedStore}>
        <NowPlayingPanel />
      </Provider>,
    )

    // Advance past the debounce window
    vi.advanceTimersByTime(500)

    // Should only make one call despite both serverUp and streamReconnected changing
    expect(subsonic.getNowPlaying).toHaveBeenCalledTimes(1)

    vi.useRealTimers()
  })

  it('skips polling when server is down', () => {
    vi.useFakeTimers()

    const store = createMockStore({
      nowPlayingCount: 1,
      serverStart: { startTime: null }, // Server is down
    })
    render(
      <Provider store={store}>
        <NowPlayingPanel />
      </Provider>,
    )

    // Clear initial mount fetch
    vi.clearAllMocks()

    // Advance time by 70 seconds to trigger polling interval
    vi.advanceTimersByTime(70000)

    // Should not have made any additional requests due to server being down
    expect(subsonic.getNowPlaying).not.toHaveBeenCalled()

    vi.useRealTimers()
  })

  it('resumes polling when server comes back up', () => {
    vi.useFakeTimers()

    const store = createMockStore({
      nowPlayingCount: 1,
      serverStart: { startTime: null }, // Server is down
    })
    const { rerender } = render(
      <Provider store={store}>
        <NowPlayingPanel />
      </Provider>,
    )

    // Clear initial mount fetch
    vi.clearAllMocks()

    // Advance time - should not poll when server is down
    vi.advanceTimersByTime(70000)
    expect(subsonic.getNowPlaying).not.toHaveBeenCalled()

    // Update state to indicate server is back up
    const updatedStore = createMockStore({
      nowPlayingCount: 1,
      serverStart: { startTime: Date.now() }, // Server is back up
    })
    rerender(
      <Provider store={updatedStore}>
        <NowPlayingPanel />
      </Provider>,
    )

    // Clear the fetch that happens due to initial mount of rerender
    vi.clearAllMocks()

    // Advance time again - should now poll since server is up
    vi.advanceTimersByTime(70000)
    expect(subsonic.getNowPlaying).toHaveBeenCalled()

    vi.useRealTimers()
  })

  it('triggers takeover when clicking item or takeover button', async () => {
    const store = createMockStore()
    render(
      <Provider store={store}>
        <NowPlayingPanel />
      </Provider>,
    )

    await vi.advanceTimersByTimeAsync(500)
    fireEvent.click(screen.getByRole('button'))

    await waitFor(() => {
      expect(screen.getByText('Song')).toBeInTheDocument()
    })

    const takeoverBtn = screen.getByRole('button', {
      name: 'nowPlaying.takeoverTooltip',
    })
    expect(takeoverBtn).toBeInTheDocument()
    fireEvent.click(takeoverBtn)
  })

  it('correctly formats various player names with formatDeviceName', () => {
    expect(formatDeviceName('NavidromeUI [Chrome/macOS]')).toBe(
      'Chrome · macOS',
    )
    expect(formatDeviceName('NavidromeUI [Mobile Safari/iOS]')).toBe(
      'Mobile Safari · iOS',
    )
    expect(formatDeviceName('NavidromeUI [Edge/Windows]')).toBe(
      'Edge · Windows',
    )
    expect(formatDeviceName('NavidromeUI')).toBe('网页端')
    expect(formatDeviceName('Chora [Android]')).toBe('Chora (Android)')
    expect(formatDeviceName('MacBook Pro')).toBe('MacBook Pro')
    expect(formatDeviceName('')).toBe('')
  })

  it('renders current device badge and avoids takeover button for local session', async () => {
    subsonic.getNowPlaying.mockResolvedValue({
      json: {
        'subsonic-response': {
          status: 'ok',
          nowPlaying: {
            entry: [
              {
                playerId: 1,
                sessionId: 'test-client-unique-id', // matches mocked clientUniqueId
                username: 'u1',
                playerName: 'NavidromeUI [Chrome/macOS]',
                title: 'Local Song',
                albumArtist: 'Local Artist',
                albumId: 'album1',
                duration: 200,
                positionMs: 50000,
              },
            ],
          },
        },
      },
    })

    const store = createMockStore()
    render(
      <Provider store={store}>
        <NowPlayingPanel />
      </Provider>,
    )

    await vi.advanceTimersByTimeAsync(500)
    fireEvent.click(screen.getByRole('button'))

    await waitFor(() => {
      expect(screen.getByText('Local Song')).toBeInTheDocument()
      expect(screen.getByText('u1 (Chrome · macOS)')).toBeInTheDocument()
      // Current device chips
      expect(
        screen.getAllByText('nowPlaying.currentDeviceShort').length,
      ).toBeGreaterThan(0)
    })

    // Should not render takeover button for local session
    expect(
      screen.queryByRole('button', { name: 'nowPlaying.takeoverTooltip' }),
    ).not.toBeInTheDocument()
  })

  it('renders remote output and volume badges and passes inherited properties on takeover', async () => {
    subsonic.getNowPlaying.mockResolvedValue({
      json: {
        'subsonic-response': {
          status: 'ok',
          nowPlaying: {
            entry: [
              {
                playerId: 'remote-client-1',
                sessionId: 'remote-client-1',
                username: 'alice',
                playerName: 'NavidromeUI [Chrome/Linux]',
                title: 'Heartless',
                albumArtist: 'Futuristic Swaver',
                albumId: 'album-1',
                duration: 163,
                positionMs: 51000,
                state: 'paused',
                outputDevice: 'xiaomi_l7a',
                volume: 65,
                playMode: 'single',
                bilingual: true,
              },
            ],
          },
        },
      },
    })

    const store = createMockStore()
    render(
      <Provider store={store}>
        <NowPlayingPanel />
      </Provider>,
    )

    await vi.advanceTimersByTimeAsync(500)
    fireEvent.click(screen.getByRole('button'))

    await waitFor(() => {
      expect(screen.getByText('Heartless')).toBeInTheDocument()
      expect(screen.getByText('xiaomi_l7a')).toBeInTheDocument()
      expect(screen.getByText('65%')).toBeInTheDocument()
    })

    const takeoverBtn = screen.getByRole('button', {
      name: 'nowPlaying.takeoverTooltip',
    })
    expect(takeoverBtn).toBeInTheDocument()
    fireEvent.click(takeoverBtn)

    await waitFor(() => {
      expect(httpClient).toHaveBeenCalledWith(
        '/api/playback/sessions/remote-client-1/takeover',
        expect.objectContaining({
          method: 'POST',
          body: expect.stringContaining('"targetOutput":"xiaomi_l7a"'),
        }),
      )
    })
  })

  it('toggles closed when button is clicked again', async () => {
    const store = createMockStore()
    render(
      <Provider store={store}>
        <NowPlayingPanel />
      </Provider>,
    )

    await vi.advanceTimersByTimeAsync(500)
    const button = screen.getByRole('button')

    // Open
    fireEvent.click(button)
    await waitFor(() => {
      expect(screen.getByRole('presentation')).toBeInTheDocument()
    })

    // Click again to close
    fireEvent.click(button)
    await waitFor(() => {
      expect(screen.queryByRole('presentation')).not.toBeInTheDocument()
    })
  })
})
