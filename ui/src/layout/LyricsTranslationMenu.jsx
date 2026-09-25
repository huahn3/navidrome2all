import React, { forwardRef } from 'react'
import { MenuItemLink, useTranslate } from 'react-admin'
import { MdTranslate } from 'react-icons/md'
import { makeStyles } from '@material-ui/core'

const useStyles = makeStyles((theme) => ({
  menuItem: {
    color: theme.palette.text.secondary,
  },
}))

const LyricsTranslationMenu = forwardRef(
  ({ onClick, sidebarIsOpen, dense }, ref) => {
    const translate = useTranslate()
    const classes = useStyles()
    return (
      <MenuItemLink
        ref={ref}
        to="/lyrics-translation"
        primaryText={translate('menu.lyricsTranslation.name', {
          _: '歌词翻译',
        })}
        leftIcon={<MdTranslate size={24} />}
        onClick={onClick}
        className={classes.menuItem}
        sidebarIsOpen={sidebarIsOpen}
        dense={dense}
      />
    )
  },
)

LyricsTranslationMenu.displayName = 'LyricsTranslationMenu'

export default LyricsTranslationMenu
