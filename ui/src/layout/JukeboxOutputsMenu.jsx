import React, { forwardRef } from 'react'
import { MenuItemLink, useTranslate } from 'react-admin'
import { MdSpeaker } from 'react-icons/md'
import { makeStyles } from '@material-ui/core'

const useStyles = makeStyles((theme) => ({
  menuItem: {
    color: theme.palette.text.secondary,
  },
}))

// Sits with the other top-level entries in the user menu (next to 个性化 and
// 歌词翻译) instead of in the react-admin resource list, because the console is
// a custom page, not a resource.
const JukeboxOutputsMenu = forwardRef(
  ({ onClick, sidebarIsOpen, dense }, ref) => {
    const translate = useTranslate()
    const classes = useStyles()
    return (
      <MenuItemLink
        ref={ref}
        to="/jukebox-outputs"
        primaryText={translate('menu.jukebox.name', { _: '输出设备' })}
        leftIcon={<MdSpeaker size={24} />}
        onClick={onClick}
        className={classes.menuItem}
        sidebarIsOpen={sidebarIsOpen}
        dense={dense}
      />
    )
  },
)

JukeboxOutputsMenu.displayName = 'JukeboxOutputsMenu'

export default JukeboxOutputsMenu
