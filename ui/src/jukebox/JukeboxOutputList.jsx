import React from 'react'
import {
  Datagrid,
  DeleteButton,
  EditButton,
  FunctionField,
  TextField,
  useTranslate,
} from 'react-admin'
import Chip from '@material-ui/core/Chip'
import { makeStyles } from '@material-ui/core/styles'
import { useMediaQuery } from '@material-ui/core'
import { SimpleList, List } from '../common'

const useStyles = makeStyles((theme) => ({
  chip: {
    marginLeft: theme.spacing(0.5),
    height: 20,
  },
}))

const SourceField = ({ record }) => {
  const classes = useStyles()
  const translate = useTranslate()
  if (!record) {
    return null
  }
  const isConfig = record.source === 'config'
  return (
    <Chip
      className={classes.chip}
      size="small"
      color={isConfig ? 'default' : 'primary'}
      variant={isConfig ? 'outlined' : 'default'}
      label={translate(
        isConfig
          ? 'resources.jukeboxOutput.sourceValues.config'
          : 'resources.jukeboxOutput.sourceValues.ui',
      )}
    />
  )
}

const OutputActions = ({ record }) =>
  record && record.source === 'config' ? <EditButton record={record} /> : null

const JukeboxOutputList = (props) => {
  const isXsmall = useMediaQuery((theme) => theme.breakpoints.down('xs'))
  return (
    <List {...props} exporter={false}>
      {isXsmall ? (
        <SimpleList
          primaryText={(r) => r.name}
          secondaryText={(r) => `${r.type} · ${r.address}`}
          tertiaryText={(r) => <SourceField record={r} />}
        />
      ) : (
        <Datagrid rowClick="edit">
          <TextField source="name" />
          <TextField source="id" />
          <TextField source="type" />
          <TextField source="address" />
          <SourceField
            source="source"
            label="resources.jukeboxOutput.fields.source"
          />
          <OutputActions />
          <FunctionField
            label=""
            render={(record) =>
              record.source === 'config' ? null : (
                <DeleteButton record={record} />
              )
            }
          />
        </Datagrid>
      )}
    </List>
  )
}

export default JukeboxOutputList
