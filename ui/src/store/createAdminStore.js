import {
  applyMiddleware,
  combineReducers,
  compose,
  legacy_createStore as createStore,
} from 'redux'
import { routerMiddleware, connectRouter } from 'connected-react-router'
import createSagaMiddleware from 'redux-saga'
import { all, fork } from 'redux-saga/effects'
import { adminReducer, adminSaga, USER_LOGOUT } from 'react-admin'
import throttle from 'lodash.throttle'
import config from '../config'
import { loadState, saveState } from './persistState'

const createAdminStore = ({
  authProvider,
  dataProvider,
  history,
  customReducers = {},
}) => {
  const reducer = combineReducers({
    admin: adminReducer,
    router: connectRouter(history),
    ...customReducers,
  })
  const resettableAppReducer = (state, action) =>
    reducer(action.type !== USER_LOGOUT ? state : undefined, action)

  const saga = function* rootSaga() {
    yield all([adminSaga(dataProvider, authProvider)].map(fork))
  }
  const sagaMiddleware = createSagaMiddleware()

  const composeEnhancers =
    (process.env.NODE_ENV === 'development' &&
      typeof window !== 'undefined' &&
      window.__REDUX_DEVTOOLS_EXTENSION_COMPOSE__ &&
      window.__REDUX_DEVTOOLS_EXTENSION_COMPOSE__({
        trace: true,
        traceLimit: 25,
      })) ||
    compose

  const persistedState = loadState()
  if (persistedState?.player?.savedPlayIndex) {
    persistedState.player.playIndex = persistedState.player.savedPlayIndex
  }
  // Muting stores 0, and the level to unmute back to lives only in memory, so a
  // persisted 0 would resurrect a silent player on every reload. Ignore it.
  if (persistedState?.player && !persistedState.player.volume) {
    persistedState.player.volume = config.defaultUIVolume / 100
  }
  const store = createStore(
    resettableAppReducer,
    persistedState,
    composeEnhancers(
      applyMiddleware(sagaMiddleware, routerMiddleware(history)),
    ),
  )

  store.subscribe(
    throttle(() => {
      const state = store.getState()
      saveState({
        theme: state.theme,
        library: state.library,
        player: (({ queue, volume, savedPlayIndex, outputDevice }) => ({
          queue,
          volume,
          savedPlayIndex,
          outputDevice,
        }))(state.player),
        albumView: state.albumView,
        settings: state.settings,
      })
    }),
    1000,
  )

  sagaMiddleware.run(saga)
  return store
}

export default createAdminStore
