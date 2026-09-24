import { makeStyles } from '@material-ui/core/styles'

const useStyle = makeStyles(
  (theme) => {
    const isDark = theme.palette.type === 'dark'
    return {
      audioTitle: {
        textDecoration: 'none',
        color: theme.palette.primary.dark,
        display: 'block',
        minWidth: 0,
      },
      songTitle: {
        fontWeight: 'bold',
        '&:hover + $qualityInfo': {
          opacity: 1,
        },
      },
      songInfo: {
        display: 'block',
        marginTop: '2px',
      },
      songAlbum: {
        fontStyle: 'italic',
        fontSize: 'smaller',
      },
      qualityInfo: {
        marginTop: '-4px',
        opacity: 0,
        transition: 'all 500ms ease-out',
      },
      player: {
        display: (props) => (props.visible ? 'block' : 'none'),

        // Completely hide the floating mini circle controller if it ever renders
        '& .react-jinke-music-player:not(.react-jinke-music-player-main)': {
          display: 'none !important',
        },
        '& .music-player-controller': {
          display: 'none !important',
        },
        '& .audio-circle-process-bar': {
          display: 'none !important',
        },
        // Completely hide the old full-screen mobile overlay
        '& .react-jinke-music-player-mobile': {
          display: 'none !important',
        },

        // VolumeControl replaces the player's built-in slider, so there is only
        // one volume UI on every screen size
        '& .music-player-panel .panel-content .player-content .play-sounds': {
          display: 'none !important',
        },

        '@media (prefers-reduced-motion)': {
          '& .music-player-panel .panel-content div.img-rotate': {
            animation: 'none',
          },
        },

        '& .play-mode-title': {
          pointerEvents: 'none',
        },

        // Modern Dock Bar Container
        '& .music-player-panel': {
          background: isDark
            ? 'rgba(20, 24, 36, 0.88) !important'
            : 'rgba(255, 255, 255, 0.92) !important',
          backdropFilter: 'blur(20px) saturate(180%) !important',
          WebkitBackdropFilter: 'blur(20px) saturate(180%) !important',
          borderTop: `1px solid ${
            isDark ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.08)'
          } !important`,
          boxShadow: isDark
            ? '0 -4px 24px rgba(0, 0, 0, 0.45) !important'
            : '0 -4px 20px rgba(0, 0, 0, 0.08) !important',
          color: `${theme.palette.text.primary} !important`,
          transition: 'all 0.3s ease !important',
          height: '76px',
          zIndex: 1000,
          WebkitTapHighlightColor: 'transparent !important',
          WebkitTouchCallout: 'none !important',
          userSelect: 'none !important',
          WebkitUserSelect: 'none !important',
          MozUserSelect: 'none !important',
          msUserSelect: 'none !important',
          outline: 'none !important',
          '& *': {
            WebkitTapHighlightColor: 'transparent !important',
            outline: 'none !important',
          },
          '& ::selection': {
            background: 'transparent !important',
          },
          '& .MuiTouchRipple-root': {
            display: 'none !important',
          },
        },

        '& .music-player-panel .panel-content': {
          display: 'flex',
          alignItems: 'center',
          height: '100%',
          padding: '0 24px',
          position: 'relative',
        },

        // Album Art
        '& .music-player-panel .panel-content div.img-content': {
          animationDuration: (props) => !props.enableCoverAnimation && '0s',
          borderRadius: (props) =>
            !props.enableCoverAnimation ? '8px !important' : '50% !important',
          backgroundSize: 'cover',
          backgroundPosition: 'center',
          boxShadow: '0 2px 10px rgba(0, 0, 0, 0.28)',
          width: '48px !important',
          height: '48px !important',
          flexShrink: 0,
          transition: 'transform 0.2s ease',
          '&:hover': {
            transform: 'scale(1.04)',
          },
        },

        // Track Info & Progress Bar
        '& .music-player-panel .panel-content .progress-bar-content': {
          display: 'flex !important', // Ensure it is NEVER hidden by the library's default media query
          flexDirection: 'column',
          flex: '1 1 auto',
          minWidth: 0,
          padding: '0 16px',
          overflow: 'hidden',
        },
        '& .music-player-panel .panel-content .progress-bar-content .audio-title':
          {
            display: 'block',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            lineHeight: 1.3,
            fontSize: '13px',
          },
        '& .music-player-panel .panel-content .progress-bar-content section.audio-main':
          {
            display: (props) =>
              props.isRadio ? 'none !important' : 'inline-flex !important',
            alignItems: 'center',
            marginTop: '3px',
            width: '100%',
          },
        '& .music-player-panel .panel-content .progress-bar-content section.audio-main .current-time, & .music-player-panel .panel-content .progress-bar-content section.audio-main .duration':
          {
            fontSize: '11px !important',
            fontVariantNumeric: 'tabular-nums',
            opacity: 0.65,
            flex: '0 0 auto',
            width: '34px',
            textAlign: 'center',
          },
        '& .music-player-panel .panel-content .progress-bar-content section.audio-main .progress-bar':
          {
            flex: '1 1 auto',
            margin: '0 8px !important',
            height: '14px',
            display: 'flex',
            alignItems: 'center',
            position: 'relative',
          },

        // Sliders (progress bar & volume)
        '& .rc-slider-rail': {
          backgroundColor: isDark
            ? 'rgba(255, 255, 255, 0.18) !important'
            : 'rgba(0, 0, 0, 0.15) !important',
          height: '4px !important',
          borderRadius: '2px !important',
        },
        '& .rc-slider-track': {
          backgroundColor: `${theme.palette.primary.main} !important`,
          height: '4px !important',
          borderRadius: '2px !important',
        },
        '& .rc-slider-handle': {
          backgroundColor: '#fff !important',
          border: `2px solid ${theme.palette.primary.main} !important`,
          width: '12px !important',
          height: '12px !important',
          marginTop: '-4px !important',
          boxShadow: '0 1px 4px rgba(0,0,0,0.3) !important',
          transition:
            'transform 0.15s ease, border-color 0.15s ease !important',
          '&:hover, &:active': {
            transform: 'scale(1.25)',
            borderColor: `${theme.palette.primary.light || theme.palette.primary.main} !important`,
          },
        },

        // Player Controls & Icons
        '& .music-player-panel .panel-content .player-content': {
          display: 'inline-flex',
          alignItems: 'center',
          flex: '0 0 auto',
          paddingLeft: '12px',
          gap: '4px',
        },
        '& .music-player-panel .panel-content .player-content svg': {
          fontSize: '22px',
          color: `${theme.palette.text.primary} !important`,
          opacity: 0.85,
          transition: 'opacity 0.2s, color 0.2s, transform 0.15s',
          '&:hover': {
            opacity: 1,
            color: `${theme.palette.primary.main} !important`,
          },
        },
        '& .music-player-panel .panel-content .player-content .lyric-btn svg': {
          fontSize: '17px !important',
          width: '17px !important',
          height: '17px !important',
        },
        '& .music-player-panel .panel-content .player-content .prev-audio svg, & .music-player-panel .panel-content .player-content .next-audio svg':
          {
            fontSize: '24px',
          },
        '& .music-player-panel .panel-content .player-content .play-btn': {
          padding: '0 6px',
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'center',
        },
        '& .music-player-panel .panel-content .player-content .play-btn svg': {
          fontSize: '26px',
          opacity: 1,
        },
        '& .music-player-panel .panel-content .player-content .audio-lists-btn':
          {
            backgroundColor: isDark
              ? 'rgba(255, 255, 255, 0.08) !important'
              : 'rgba(0, 0, 0, 0.06) !important',
            borderRadius: '14px !important',
            height: '26px !important',
            minWidth: '50px !important',
            padding: '0 8px !important',
            border: `1px solid ${
              isDark ? 'rgba(255, 255, 255, 0.12)' : 'rgba(0, 0, 0, 0.08)'
            }`,
            color: `${theme.palette.text.primary} !important`,
            display: 'inline-flex !important',
            alignItems: 'center !important',
            justifyContent: 'center !important',
            boxShadow: 'none !important',
            '&:hover': {
              backgroundColor: `${theme.palette.primary.main}22 !important`,
              color: `${theme.palette.primary.main} !important`,
            },
          },
        '& .music-player-panel .panel-content .player-content .destroy-btn': {
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'center',
          cursor: 'pointer',
          padding: '0 4px',
          opacity: 0.65,
          transition: 'opacity 0.2s, transform 0.2s',
          '&:hover': {
            opacity: 1,
            transform: 'scale(1.1)',
          },
          '& svg': {
            fontSize: '20px',
          },
        },

        // Responsive Breakpoint 1: Narrow Desktop & Tablet (<= 810px)
        // Two rows of icons: Row 1 = Controls + Volume, Row 2 = Tools
        '@media screen and (max-width: 810px)': {
          '& .music-player-panel': {
            height: '88px !important',
          },
          '& .music-player-panel .panel-content': {
            padding: '0 16px',
          },
          '& .music-player-panel .panel-content .player-content': {
            display: 'flex !important',
            flexWrap: 'wrap !important',
            alignItems: 'center !important',
            justifyContent: 'space-between !important',
            paddingLeft: '8px',
            rowGap: '6px',
            maxWidth: '310px',
            flex: '0 0 auto',
          },
          '& .music-player-panel .panel-content .player-content > span.group:first-child':
            {
              flex: '0 0 auto !important',
              width: '110px !important',
              margin: '0 !important',
              padding: '0 !important',
              display: 'inline-flex !important',
              alignItems: 'center !important',
              justifyContent: 'flex-start !important',
              gap: '4px !important',
              WebkitTapHighlightColor: 'transparent !important',
              outline: 'none !important',
              userSelect: 'none !important',
              WebkitUserSelect: 'none !important',
            },
          '& .music-player-panel .panel-content .player-content .prev-audio, & .music-player-panel .panel-content .player-content .next-audio':
            {
              width: '34px !important',
              height: '34px !important',
              minWidth: '34px !important',
              maxWidth: '34px !important',
              display: 'inline-flex !important',
              alignItems: 'center !important',
              justifyContent: 'center !important',
              borderRadius: '50% !important',
              cursor: 'pointer !important',
              padding: '0 !important',
              margin: '0 !important',
              WebkitTapHighlightColor: 'transparent !important',
              outline: 'none !important',
              userSelect: 'none !important',
              WebkitUserSelect: 'none !important',
              transition: 'background-color 0.15s ease !important',
              '&:hover': {
                backgroundColor: isDark
                  ? 'rgba(255, 255, 255, 0.08) !important'
                  : 'rgba(0, 0, 0, 0.06) !important',
              },
              '&:focus, &:focus-visible, &:active': {
                outline: 'none !important',
                boxShadow: 'none !important',
                WebkitTapHighlightColor: 'transparent !important',
              },
            },
          '& .music-player-panel .panel-content .player-content .prev-audio svg, & .music-player-panel .panel-content .player-content .next-audio svg':
            {
              fontSize: '20px !important',
              width: '20px !important',
              height: '20px !important',
            },
          '& .music-player-panel .panel-content .player-content .play-btn': {
            width: '36px !important',
            height: '36px !important',
            minWidth: '36px !important',
            maxWidth: '36px !important',
            display: 'inline-flex !important',
            alignItems: 'center !important',
            justifyContent: 'center !important',
            borderRadius: '50% !important',
            cursor: 'pointer !important',
            padding: '0 !important',
            margin: '0 !important',
            WebkitTapHighlightColor: 'transparent !important',
            outline: 'none !important',
            userSelect: 'none !important',
            WebkitUserSelect: 'none !important',
            backgroundColor: isDark
              ? 'rgba(255, 255, 255, 0.12) !important'
              : 'rgba(0, 0, 0, 0.06) !important',
            transition:
              'background-color 0.15s ease, transform 0.1s ease !important',
            '&:hover': {
              backgroundColor: isDark
                ? 'rgba(255, 255, 255, 0.2) !important'
                : 'rgba(0, 0, 0, 0.12) !important',
              transform: 'scale(1.06) !important',
            },
            '&:focus, &:focus-visible, &:active': {
              outline: 'none !important',
              boxShadow: 'none !important',
              WebkitTapHighlightColor: 'transparent !important',
            },
          },
          '& .music-player-panel .panel-content .player-content .play-btn svg':
            {
              fontSize: '24px !important',
              width: '24px !important',
              height: '24px !important',
              color: `${theme.palette.primary.main} !important`,
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-row"]':
            {
              flex: '1 1 calc(100% - 118px) !important',
              width: 'calc(100% - 118px) !important',
              maxWidth: 'calc(100% - 118px) !important',
              margin: '0 !important',
              padding: '0 0 0 6px !important',
              display: 'flex !important',
              alignItems: 'center !important',
              justifyContent: 'flex-end !important',
              height: '36px !important',
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"]':
            {
              width: '100% !important',
              maxWidth: '200px !important',
              display: 'flex !important',
              alignItems: 'center !important',
              justifyContent: 'flex-end !important',
              gap: '4px !important',
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] .MuiSlider-root':
            {
              flex: '1 1 auto !important',
              minWidth: '60px !important',
              margin: '0 6px !important',
              width: 'auto !important',
              height: '4px !important',
              padding: '12px 0 !important',
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] .MuiSlider-rail':
            {
              height: '4px !important',
              borderRadius: '2px !important',
              opacity: '1 !important',
              backgroundColor: isDark
                ? 'rgba(255, 255, 255, 0.2) !important'
                : 'rgba(0, 0, 0, 0.15) !important',
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] .MuiSlider-track':
            {
              height: '4px !important',
              borderRadius: '2px !important',
              backgroundColor: `${theme.palette.primary.main} !important`,
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] .MuiSlider-thumb':
            {
              width: '12px !important',
              height: '12px !important',
              minWidth: '12px !important',
              maxWidth: '12px !important',
              minHeight: '12px !important',
              maxHeight: '12px !important',
              borderRadius: '50% !important',
              boxSizing: 'border-box !important',
              marginTop: '-4px !important',
              marginLeft: '-6px !important',
              backgroundColor: '#ffffff !important',
              border: `2px solid ${theme.palette.primary.main} !important`,
              boxShadow: '0 1px 3px rgba(0,0,0,0.35) !important',
              outline: 'none !important',
              WebkitTapHighlightColor: 'transparent !important',
              transition: 'transform 0.12s ease !important',
              '&:hover, &.Mui-focusVisible, &:active, &.MuiSlider-active': {
                boxShadow: 'none !important',
                transform: 'scale(1.15) !important',
              },
              '&::before, &::after': {
                display: 'none !important',
              },
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] .MuiSlider-valueLabel':
            {
              display: 'none !important',
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] button':
            {
              width: '34px !important',
              height: '34px !important',
              padding: '0 !important',
              margin: '0 !important',
              flex: '0 0 34px !important',
              display: 'inline-flex !important',
              alignItems: 'center !important',
              justifyContent: 'center !important',
              borderRadius: '50% !important',
              outline: 'none !important',
              WebkitTapHighlightColor: 'transparent !important',
              '&:focus, &:focus-visible, &:active': {
                outline: 'none !important',
                boxShadow: 'none !important',
                WebkitTapHighlightColor: 'transparent !important',
              },
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] svg':
            {
              fontSize: '19px !important',
              width: '19px !important',
              height: '19px !important',
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] [data-testid="volume-percent"]':
            {
              fontSize: '11px !important',
              fontVariantNumeric: 'tabular-nums !important',
              opacity: '0.8 !important',
              width: '30px !important',
              textAlign: 'right !important',
              flex: '0 0 30px !important',
              userSelect: 'none !important',
              WebkitUserSelect: 'none !important',
            },
          '& .music-player-panel .panel-content .player-content > .group:not(:first-child)':
            {
              flex: '0 0 auto !important',
              margin: '0 !important',
            },
          '& .music-player-panel .panel-content .player-content > li:not([data-testid="volume-row"]), & .music-player-panel .panel-content .player-content > .loop-btn, & .music-player-panel .panel-content .player-content > .lyric-btn, & .music-player-panel .panel-content .player-content > .audio-lists-btn, & .music-player-panel .panel-content .player-content > .destroy-btn':
            {
              flex: '0 0 34px !important',
              width: '34px !important',
              height: '34px !important',
              minWidth: '34px !important',
              maxWidth: '34px !important',
              margin: '0 !important',
              padding: '0 !important',
              display: 'inline-flex !important',
              alignItems: 'center !important',
              justifyContent: 'center !important',
              borderRadius: '50% !important',
              cursor: 'pointer !important',
              background: 'transparent !important',
              backgroundColor: 'transparent !important',
              border: 'none !important',
              boxShadow: 'none !important',
              opacity: '0.85 !important',
              outline: 'none !important',
              WebkitTapHighlightColor: 'transparent !important',
              userSelect: 'none !important',
              WebkitUserSelect: 'none !important',
              transition:
                'background-color 0.15s ease, opacity 0.15s ease !important',
              '&:hover': {
                opacity: '1 !important',
                backgroundColor: isDark
                  ? 'rgba(255, 255, 255, 0.08) !important'
                  : 'rgba(0, 0, 0, 0.06) !important',
              },
              '&:focus, &:focus-visible, &:active': {
                outline: 'none !important',
                boxShadow: 'none !important',
                WebkitTapHighlightColor: 'transparent !important',
              },
            },
          '& .music-player-panel .panel-content .player-content > li:not([data-testid="volume-row"]) button, & .music-player-panel .panel-content .player-content > li:not([data-testid="volume-row"]) .MuiIconButton-root':
            {
              width: '34px !important',
              height: '34px !important',
              minWidth: '34px !important',
              maxWidth: '34px !important',
              padding: '0 !important',
              margin: '0 !important',
              display: 'inline-flex !important',
              alignItems: 'center !important',
              justifyContent: 'center !important',
              borderRadius: '50% !important',
              background: 'transparent !important',
              border: 'none !important',
              boxShadow: 'none !important',
              outline: 'none !important',
              WebkitTapHighlightColor: 'transparent !important',
              '&:focus, &:focus-visible, &:active': {
                outline: 'none !important',
                boxShadow: 'none !important',
                WebkitTapHighlightColor: 'transparent !important',
              },
            },
          '& .music-player-panel .panel-content .player-content > li:not([data-testid="volume-row"]) svg, & .music-player-panel .panel-content .player-content > .loop-btn svg, & .music-player-panel .panel-content .player-content > .lyric-btn svg, & .music-player-panel .panel-content .player-content > .audio-lists-btn svg, & .music-player-panel .panel-content .player-content > .destroy-btn svg':
            {
              fontSize: '19px !important',
              width: '19px !important',
              height: '19px !important',
              minWidth: '19px !important',
              maxWidth: '19px !important',
              minHeight: '19px !important',
              maxHeight: '19px !important',
              display: 'block !important',
              margin: '0 auto !important',
              padding: '0 !important',
              boxSizing: 'border-box !important',
              userSelect: 'none !important',
              WebkitUserSelect: 'none !important',
            },
          '& .music-player-panel .panel-content .player-content > .lyric-btn svg':
            {
              fontSize: '14px !important',
              width: '14px !important',
              height: '14px !important',
              minWidth: '14px !important',
              maxWidth: '14px !important',
              minHeight: '14px !important',
              maxHeight: '14px !important',
              display: 'block !important',
              margin: '0 auto !important',
              padding: '0 !important',
            },
          '& .music-player-panel .panel-content .player-content > .audio-lists-btn .audio-lists-num':
            {
              display: 'none !important',
            },
          '& .music-player-panel .panel-content .player-content > .audio-lists-btn .audio-lists-icon':
            {
              display: 'inline-flex !important',
              alignItems: 'center !important',
              justifyContent: 'center !important',
              width: '19px !important',
              height: '19px !important',
              margin: '0 !important',
              padding: '0 !important',
            },
        },

        // Responsive Breakpoint 2: Mobile Portrait (<= 600px)
        // Stacked layout: Top = Cover + Title/Progress, Bottom = Two rows of icons
        '@media screen and (max-width: 600px)': {
          '& .music-player-panel': {
            height: 'auto !important',
            minHeight: '136px',
            padding:
              '8px 12px calc(8px + env(safe-area-inset-bottom)) !important',
          },
          '& .music-player-panel .panel-content': {
            display: 'grid',
            gridTemplateColumns: 'auto 1fr',
            gridTemplateRows: 'auto auto',
            rowGap: '8px',
            columnGap: '10px',
            padding: '0 !important',
            height: 'auto',
            alignItems: 'center',
          },
          '& .music-player-panel .panel-content div.img-content': {
            gridColumn: '1',
            gridRow: '1',
            width: '42px !important',
            height: '42px !important',
            margin: '0 !important',
            alignSelf: 'center',
          },
          '& .music-player-panel .panel-content .progress-bar-content': {
            gridColumn: '2',
            gridRow: '1',
            padding: '0 !important',
            minWidth: 0,
          },
          '& .music-player-panel .panel-content .player-content': {
            gridColumn: '1 / -1',
            gridRow: '2',
            width: '100%',
            maxWidth: '100% !important',
            padding: '0 !important',
            display: 'flex !important',
            flexWrap: 'wrap !important',
            justifyContent: 'space-between !important',
            alignItems: 'center !important',
            rowGap: '6px',
          },
          '& .music-player-panel .panel-content .player-content > span.group:first-child':
            {
              flex: '0 0 auto !important',
              width: '112px !important',
              margin: '0 !important',
              padding: '0 !important',
              display: 'inline-flex !important',
              alignItems: 'center !important',
              justifyContent: 'flex-start !important',
              gap: '6px !important',
              WebkitTapHighlightColor: 'transparent !important',
              outline: 'none !important',
              userSelect: 'none !important',
              WebkitUserSelect: 'none !important',
            },
          '& .music-player-panel .panel-content .player-content .prev-audio, & .music-player-panel .panel-content .player-content .next-audio':
            {
              width: '34px !important',
              height: '34px !important',
              minWidth: '34px !important',
              maxWidth: '34px !important',
              display: 'inline-flex !important',
              alignItems: 'center !important',
              justifyContent: 'center !important',
              borderRadius: '50% !important',
              cursor: 'pointer !important',
              padding: '0 !important',
              margin: '0 !important',
              WebkitTapHighlightColor: 'transparent !important',
              outline: 'none !important',
              userSelect: 'none !important',
              WebkitUserSelect: 'none !important',
              transition: 'background-color 0.15s ease !important',
              '&:hover': {
                backgroundColor: isDark
                  ? 'rgba(255, 255, 255, 0.08) !important'
                  : 'rgba(0, 0, 0, 0.06) !important',
              },
              '&:focus, &:focus-visible, &:active': {
                outline: 'none !important',
                boxShadow: 'none !important',
                WebkitTapHighlightColor: 'transparent !important',
              },
              '& svg': {
                fontSize: '20px !important',
                width: '20px !important',
                height: '20px !important',
              },
            },
          '& .music-player-panel .panel-content .player-content .play-btn': {
            width: '36px !important',
            height: '36px !important',
            minWidth: '36px !important',
            maxWidth: '36px !important',
            display: 'inline-flex !important',
            alignItems: 'center !important',
            justifyContent: 'center !important',
            borderRadius: '50% !important',
            cursor: 'pointer !important',
            padding: '0 !important',
            margin: '0 !important',
            WebkitTapHighlightColor: 'transparent !important',
            outline: 'none !important',
            userSelect: 'none !important',
            WebkitUserSelect: 'none !important',
            backgroundColor: isDark
              ? 'rgba(255, 255, 255, 0.12) !important'
              : 'rgba(0, 0, 0, 0.06) !important',
            transition:
              'background-color 0.15s ease, transform 0.1s ease !important',
            '&:hover': {
              backgroundColor: isDark
                ? 'rgba(255, 255, 255, 0.2) !important'
                : 'rgba(0, 0, 0, 0.12) !important',
              transform: 'scale(1.06) !important',
            },
            '&:focus, &:focus-visible, &:active': {
              outline: 'none !important',
              boxShadow: 'none !important',
              WebkitTapHighlightColor: 'transparent !important',
            },
            '& svg': {
              fontSize: '24px !important',
              width: '24px !important',
              height: '24px !important',
              color: `${theme.palette.primary.main} !important`,
            },
          },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-row"]':
            {
              flex: '1 1 calc(100% - 124px) !important',
              width: 'calc(100% - 124px) !important',
              maxWidth: 'calc(100% - 124px) !important',
              minWidth: '130px !important',
              margin: '0 !important',
              padding: '0 0 0 8px !important',
              display: 'flex !important',
              alignItems: 'center !important',
              justifyContent: 'flex-end !important',
              height: '36px !important',
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"]':
            {
              width: '100% !important',
              maxWidth: '220px !important',
              display: 'flex !important',
              alignItems: 'center !important',
              justifyContent: 'flex-end !important',
              gap: '4px !important',
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] .MuiSlider-root':
            {
              flex: '1 1 auto !important',
              minWidth: '60px !important',
              margin: '0 8px !important',
              width: 'auto !important',
              height: '4px !important',
              padding: '12px 0 !important',
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] .MuiSlider-rail':
            {
              height: '4px !important',
              borderRadius: '2px !important',
              opacity: '1 !important',
              backgroundColor: isDark
                ? 'rgba(255, 255, 255, 0.2) !important'
                : 'rgba(0, 0, 0, 0.15) !important',
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] .MuiSlider-track':
            {
              height: '4px !important',
              borderRadius: '2px !important',
              backgroundColor: `${theme.palette.primary.main} !important`,
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] .MuiSlider-thumb':
            {
              width: '12px !important',
              height: '12px !important',
              minWidth: '12px !important',
              maxWidth: '12px !important',
              minHeight: '12px !important',
              maxHeight: '12px !important',
              borderRadius: '50% !important',
              boxSizing: 'border-box !important',
              marginTop: '-4px !important',
              marginLeft: '-6px !important',
              backgroundColor: '#ffffff !important',
              border: `2px solid ${theme.palette.primary.main} !important`,
              boxShadow: '0 1px 3px rgba(0,0,0,0.35) !important',
              outline: 'none !important',
              WebkitTapHighlightColor: 'transparent !important',
              transition: 'transform 0.12s ease !important',
              '&:hover, &.Mui-focusVisible, &:active, &.MuiSlider-active': {
                boxShadow: 'none !important',
                transform: 'scale(1.15) !important',
              },
              '&::before, &::after': {
                display: 'none !important',
              },
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] .MuiSlider-valueLabel':
            {
              display: 'none !important',
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] button':
            {
              width: '34px !important',
              height: '34px !important',
              padding: '0 !important',
              margin: '0 !important',
              flex: '0 0 34px !important',
              display: 'inline-flex !important',
              alignItems: 'center !important',
              justifyContent: 'center !important',
              borderRadius: '50% !important',
              outline: 'none !important',
              WebkitTapHighlightColor: 'transparent !important',
              '&:focus, &:focus-visible, &:active': {
                outline: 'none !important',
                boxShadow: 'none !important',
                WebkitTapHighlightColor: 'transparent !important',
              },
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] svg':
            {
              fontSize: '19px !important',
              width: '19px !important',
              height: '19px !important',
            },
          '& .music-player-panel .panel-content .player-content [data-testid="volume-control"] [data-testid="volume-percent"]':
            {
              fontSize: '11px !important',
              fontVariantNumeric: 'tabular-nums !important',
              opacity: '0.8 !important',
              width: '32px !important',
              textAlign: 'right !important',
              flex: '0 0 32px !important',
              userSelect: 'none !important',
              WebkitUserSelect: 'none !important',
            },
          '& .music-player-panel .panel-content .player-content > .group:not(:first-child)':
            {
              flex: '0 0 auto !important',
              margin: '0 !important',
            },
          '& .music-player-panel .panel-content .player-content > li:not([data-testid="volume-row"]), & .music-player-panel .panel-content .player-content > .loop-btn, & .music-player-panel .panel-content .player-content > .lyric-btn, & .music-player-panel .panel-content .player-content > .audio-lists-btn, & .music-player-panel .panel-content .player-content > .destroy-btn':
            {
              flex: '0 0 34px !important',
              width: '34px !important',
              height: '34px !important',
              minWidth: '34px !important',
              maxWidth: '34px !important',
              margin: '0 !important',
              padding: '0 !important',
              display: 'inline-flex !important',
              alignItems: 'center !important',
              justifyContent: 'center !important',
              borderRadius: '50% !important',
              cursor: 'pointer !important',
              background: 'transparent !important',
              backgroundColor: 'transparent !important',
              border: 'none !important',
              boxShadow: 'none !important',
              opacity: '0.85 !important',
              outline: 'none !important',
              WebkitTapHighlightColor: 'transparent !important',
              userSelect: 'none !important',
              WebkitUserSelect: 'none !important',
              transition:
                'background-color 0.15s ease, opacity 0.15s ease !important',
              '&:hover': {
                opacity: '1 !important',
                backgroundColor: isDark
                  ? 'rgba(255, 255, 255, 0.08) !important'
                  : 'rgba(0, 0, 0, 0.06) !important',
              },
              '&:focus, &:focus-visible, &:active': {
                outline: 'none !important',
                boxShadow: 'none !important',
                WebkitTapHighlightColor: 'transparent !important',
              },
            },
          '& .music-player-panel .panel-content .player-content > li:not([data-testid="volume-row"]) button, & .music-player-panel .panel-content .player-content > li:not([data-testid="volume-row"]) .MuiIconButton-root':
            {
              width: '34px !important',
              height: '34px !important',
              minWidth: '34px !important',
              maxWidth: '34px !important',
              padding: '0 !important',
              margin: '0 !important',
              display: 'inline-flex !important',
              alignItems: 'center !important',
              justifyContent: 'center !important',
              borderRadius: '50% !important',
              background: 'transparent !important',
              border: 'none !important',
              boxShadow: 'none !important',
              outline: 'none !important',
              WebkitTapHighlightColor: 'transparent !important',
              '&:focus, &:focus-visible, &:active': {
                outline: 'none !important',
                boxShadow: 'none !important',
                WebkitTapHighlightColor: 'transparent !important',
              },
            },
          '& .music-player-panel .panel-content .player-content > li:not([data-testid="volume-row"]) svg, & .music-player-panel .panel-content .player-content > .loop-btn svg, & .music-player-panel .panel-content .player-content > .lyric-btn svg, & .music-player-panel .panel-content .player-content > .audio-lists-btn svg, & .music-player-panel .panel-content .player-content > .destroy-btn svg':
            {
              fontSize: '19px !important',
              width: '19px !important',
              height: '19px !important',
              minWidth: '19px !important',
              maxWidth: '19px !important',
              minHeight: '19px !important',
              maxHeight: '19px !important',
              display: 'block !important',
              margin: '0 auto !important',
              padding: '0 !important',
              boxSizing: 'border-box !important',
              userSelect: 'none !important',
              WebkitUserSelect: 'none !important',
            },
          '& .music-player-panel .panel-content .player-content > .lyric-btn svg':
            {
              fontSize: '14px !important',
              width: '14px !important',
              height: '14px !important',
              minWidth: '14px !important',
              maxWidth: '14px !important',
              minHeight: '14px !important',
              maxHeight: '14px !important',
              display: 'block !important',
              margin: '0 auto !important',
              padding: '0 !important',
            },
          '& .music-player-panel .panel-content .player-content > .audio-lists-btn .audio-lists-num':
            {
              display: 'none !important',
            },
          '& .music-player-panel .panel-content .player-content > .audio-lists-btn .audio-lists-icon':
            {
              display: 'inline-flex !important',
              alignItems: 'center !important',
              justifyContent: 'center !important',
              width: '19px !important',
              height: '19px !important',
              margin: '0 !important',
              padding: '0 !important',
            },
        },
      },
    }
  },
  { name: 'NDAudioPlayer' },
)

export default useStyle
