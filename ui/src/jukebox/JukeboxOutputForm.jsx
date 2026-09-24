import React, { useCallback, useState } from 'react'
import {
  FormDataConsumer,
  PasswordInput,
  required,
  SelectInput,
  TextInput,
  useTranslate,
} from 'react-admin'
import { makeStyles } from '@material-ui/core/styles'
import Button from '@material-ui/core/Button'
import CircularProgress from '@material-ui/core/CircularProgress'
import List from '@material-ui/core/List'
import ListItem from '@material-ui/core/ListItem'
import ListItemText from '@material-ui/core/ListItemText'
import Typography from '@material-ui/core/Typography'
import { useForm } from 'react-final-form'
import { discoverRenderers } from '../audioplayer/jukebox'

const useScanStyles = makeStyles((theme) => ({
  scanBlock: {
    marginTop: theme.spacing(1),
    marginBottom: theme.spacing(1),
    padding: theme.spacing(1.5),
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
    width: '100%',
  },
  scanButtonRow: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
  },
  scanError: {
    marginTop: theme.spacing(1),
  },
  scanEmpty: {
    marginTop: theme.spacing(1),
  },
}))

const OUTPUT_TYPE_CHOICES = [
  { id: 'xiaomi', name: 'resources.jukeboxOutput.types.xiaomi' },
  { id: 'mpd', name: 'resources.jukeboxOutput.types.mpd' },
  { id: 'dlna', name: 'resources.jukeboxOutput.types.dlna' },
]

// JukeboxOutputForm is shared by the create and edit pages. Xiaomi/MPD specific
// fields are only shown for the matching output type.
const JukeboxOutputForm = ({ isCreate }) => (
  <>
    <TextInput
      source="id"
      validate={[required()]}
      disabled={!isCreate}
      helperText="resources.jukeboxOutput.helpers.id"
    />
    <TextInput source="name" validate={[required()]} />
    <SelectInput
      source="type"
      choices={OUTPUT_TYPE_CHOICES}
      validate={[required()]}
      translateChoice
    />
    <TextInput
      source="address"
      fullWidth
      validate={[required()]}
      helperText="resources.jukeboxOutput.helpers.address"
    />
    <FormDataConsumer>
      {({ formData }) =>
        formData.type === 'xiaomi' && (
          <>
            <TextInput
              source="token"
              fullWidth
              helperText="resources.jukeboxOutput.helpers.token"
            />
            <TextInput
              source="did"
              helperText="resources.jukeboxOutput.helpers.did"
            />
            <TextInput
              source="model"
              helperText="resources.jukeboxOutput.helpers.model"
            />
            <TextInput source="account" />
            <PasswordInput
              source="password"
              helperText="resources.jukeboxOutput.helpers.accountPassword"
            />
            <TextInput
              source="textDirective"
              helperText="resources.jukeboxOutput.helpers.textDirective"
            />
          </>
        )
      }
    </FormDataConsumer>
    <FormDataConsumer>
      {({ formData }) =>
        formData.type === 'mpd' && (
          <>
            <PasswordInput source="password" />
            <TextInput
              source="pathFrom"
              helperText="resources.jukeboxOutput.helpers.pathFrom"
            />
            <TextInput
              source="pathTo"
              helperText="resources.jukeboxOutput.helpers.pathTo"
            />
          </>
        )
      }
    </FormDataConsumer>
    <FormDataConsumer>
      {({ formData }) =>
        formData.type === 'dlna' && <DlnaScanBlock formData={formData} />
      }
    </FormDataConsumer>
  </>
)

// DlnaScanBlock runs SSDP discovery and lets the user pick a found renderer
// to fill the address (and suggest id/name) of the DLNA output being created.
const DlnaScanBlock = ({ formData }) => {
  const classes = useScanStyles()
  const translate = useTranslate()
  const form = useForm()
  const [scanning, setScanning] = useState(false)
  const [results, setResults] = useState(null)
  const [error, setError] = useState(null)
  const [scanned, setScanned] = useState(false)

  const handleScan = useCallback(() => {
    setScanning(true)
    setError(null)
    discoverRenderers(6)
      .then((devices) => {
        setResults(devices || [])
        setScanned(true)
      })
      .catch(() => {
        setError(translate('resources.jukeboxOutput.messages.scanFailed'))
        setResults([])
        setScanned(true)
      })
      .finally(() => setScanning(false))
  }, [translate])

  const handlePick = useCallback(
    (device) => () => {
      if (device.address) {
        form.change('address', device.address)
      }
      if (!formData.name && device.name) {
        form.change('name', device.name)
      }
      if (!formData.id && device.name) {
        form.change(
          'id',
          device.name
            .toLowerCase()
            .replace(/[^a-z0-9]+/g, '-')
            .replace(/^-+|-+$/g, '')
            .slice(0, 64),
        )
      }
    },
    [form, formData.id, formData.name],
  )

  return (
    <div className={classes.scanBlock}>
      <div className={classes.scanButtonRow}>
        <Button
          variant="outlined"
          color="primary"
          size="small"
          onClick={handleScan}
          disabled={scanning}
        >
          {scanning && <CircularProgress size={16} />}
          {translate('resources.jukeboxOutput.actions.scan')}
        </Button>
        <Typography variant="caption" color="textSecondary">
          {translate('resources.jukeboxOutput.helpers.scan')}
        </Typography>
      </div>
      {error && (
        <Typography variant="body2" color="error" className={classes.scanError}>
          {error}
        </Typography>
      )}
      {scanned && results && results.length === 0 && !error && (
        <Typography
          variant="body2"
          color="textSecondary"
          className={classes.scanEmpty}
        >
          {translate('resources.jukeboxOutput.messages.scanEmpty')}
        </Typography>
      )}
      {results && results.length > 0 && (
        <List dense>
          {results.map((device) => (
            <ListItem
              key={device.usn || device.address}
              button
              onClick={handlePick(device)}
            >
              <ListItemText
                primary={device.name || device.address}
                secondary={[device.model, device.address]
                  .filter(Boolean)
                  .join(' · ')}
              />
            </ListItem>
          ))}
        </List>
      )}
    </div>
  )
}

export default JukeboxOutputForm
