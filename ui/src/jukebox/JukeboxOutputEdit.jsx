import React from 'react'
import { Edit, SimpleForm, useTranslate } from 'react-admin'
import { Title } from '../common'
import JukeboxOutputForm from './JukeboxOutputForm'

const JukeboxOutputEditTitle = ({ record }) => {
  const translate = useTranslate()
  const resourceName = translate('resources.jukeboxOutput.name', {
    smart_count: 1,
  })
  return <Title subTitle={`${resourceName} ${record ? record.name : ''}`} />
}

const JukeboxOutputEdit = (props) => (
  <Edit title={<JukeboxOutputEditTitle />} {...props}>
    <SimpleForm variant={'outlined'}>
      <JukeboxOutputForm />
    </SimpleForm>
  </Edit>
)

export default JukeboxOutputEdit
