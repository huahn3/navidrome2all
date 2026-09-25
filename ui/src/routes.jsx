import React from 'react'
import { Route } from 'react-router-dom'
import Personal from './personal/Personal'
import LyricsTranslation from './lyricsTranslation/LyricsTranslation'

const routes = [
  <Route exact path="/personal" render={() => <Personal />} key={'personal'} />,
  <Route
    exact
    path="/lyrics-translation"
    render={() => <LyricsTranslation />}
    key={'lyrics-translation'}
  />,
]

export default routes
