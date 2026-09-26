import React, { Suspense, lazy } from 'react'
import { Route } from 'react-router-dom'
import Personal from './personal/Personal'
import RouteFallback from './common/RouteFallback'

// The lyrics translation console is a large, admin-only screen: loading it on
// demand keeps it out of the initial bundle.
const LyricsTranslation = lazy(
  () => import('./lyricsTranslation/LyricsTranslation'),
)
// The output console is a custom page (not a react-admin resource): the device
// cards and its own form replace the standard list/create/edit screens.
const JukeboxOutputs = lazy(() => import('./jukebox/JukeboxOutputs'))

const routes = [
  <Route exact path="/personal" render={() => <Personal />} key={'personal'} />,
  <Route
    exact
    path="/jukebox-outputs"
    render={() => (
      <Suspense fallback={<RouteFallback />}>
        <JukeboxOutputs />
      </Suspense>
    )}
    key={'jukebox-outputs'}
  />,
  <Route
    exact
    path="/lyrics-translation"
    render={() => (
      <Suspense fallback={<RouteFallback />}>
        <LyricsTranslation />
      </Suspense>
    )}
    key={'lyrics-translation'}
  />,
]

export default routes
