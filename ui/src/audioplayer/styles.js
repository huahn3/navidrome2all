import { makeStyles } from '@material-ui/core/styles'

const useStyle = makeStyles(
  (theme) => ({
    audioTitle: {
      textDecoration: 'none',
      color: theme.palette.primary.dark,
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
      // VolumeControl replaces the player's built-in slider, so there is only
      // one volume UI on every screen size
      '& .music-player-panel .panel-content .player-content .play-sounds': {
        display: 'none',
      },
      '@media (prefers-reduced-motion)': {
        '& .music-player-panel .panel-content div.img-rotate': {
          animation: 'none',
        },
      },
      '& .progress-bar-content': {
        display: 'flex',
        flexDirection: 'column',
      },
      '& .play-mode-title': {
        'pointer-events': 'none',
      },
      '& .music-player-panel .panel-content div.img-rotate': {
        // Customize desktop player when cover animation is disabled
        animationDuration: (props) => !props.enableCoverAnimation && '0s',
        borderRadius: (props) => !props.enableCoverAnimation && '0',
        // Fix cover display when image is not square
        backgroundSize: 'contain',
        backgroundPosition: 'center',
      },
      '& .react-jinke-music-player-mobile .react-jinke-music-player-mobile-cover':
        {
          // Customize mobile player when cover animation is disabled
          borderRadius: (props) => !props.enableCoverAnimation && '0',
          width: (props) => !props.enableCoverAnimation && '85%',
          maxWidth: (props) => !props.enableCoverAnimation && '600px',
          height: (props) => !props.enableCoverAnimation && 'auto',
          // Fix cover display when image is not square
          aspectRatio: '1/1',
          display: 'flex',
        },
      '& .react-jinke-music-player-mobile .react-jinke-music-player-mobile-cover img.cover':
        {
          animationDuration: (props) => !props.enableCoverAnimation && '0s',
          objectFit: 'contain', // Fix cover display when image is not square
        },
      // Hide old singer display
      '& .react-jinke-music-player-mobile .react-jinke-music-player-mobile-singer':
        {
          display: 'none',
        },
      // Hide extra whitespace from switch div
      '& .react-jinke-music-player-mobile .react-jinke-music-player-mobile-switch':
        {
          display: 'none',
        },
      '& .music-player-panel .panel-content .progress-bar-content section.audio-main':
        {
          display: (props) => (props.isRadio ? 'none' : 'inline-flex'),
        },
      '& .react-jinke-music-player-mobile-progress': {
        display: (props) => (props.isRadio ? 'none' : 'flex'),
      },
      // Mobile: let the volume row take a line of its own above the icons.
      // The selector needs to outrank the player's own `.items .item` rule.
      '& .react-jinke-music-player-mobile-operation .items': {
        flexWrap: 'wrap',
      },
      '& .react-jinke-music-player-mobile-operation .items [data-testid="volume-row"]':
        {
          flex: '1 1 100%',
          order: -1,
          height: 'auto',
          padding: '8px 12px 4px',
          justifyContent: 'flex-start',
          '& > div': {
            flex: '1 1 auto',
            minWidth: 0,
          },
        },
      '& .react-jinke-music-player-mobile-operation': {
        // Keep the icons clear of the browser/OS bar at the bottom
        paddingBottom: 'calc(12px + env(safe-area-inset-bottom))',
      },
      '& .react-jinke-music-player-mobile-operation .items .item svg': {
        color: 'rgba(255,255,255,0.85)',
      },
      // The close button sits on top of the title block
      '@media screen and (max-width:810px)': {
        '& .react-jinke-music-player-mobile-header': {
          paddingLeft: '8px',
          paddingRight: '44px',
        },
      },
    },
  }),
  { name: 'NDAudioPlayer' },
)

export default useStyle
