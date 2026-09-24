import React from 'react'
import { Create, SimpleForm, useTranslate } from 'react-admin'
import { Title } from '../common'
import JukeboxOutputForm from './JukeboxOutputForm'

const JukeboxOutputCreateTitle = () => {
  const translate = useTranslate()
  const resourceName = translate('resources.jukeboxOutput.name', {
    smart_count: 1,
  })
  const title = translate('ra.page.create', {
    name: `${resourceName}`,
  })
  return <Title subTitle={title} />
}

const JukeboxOutputCreate = (props) => (
  <Create title={<JukeboxOutputCreateTitle />} {...props}>
    <SimpleForm redirect="list" variant={'outlined'}>
      <JukeboxOutputForm isCreate />
    </SimpleForm>
  </Create>
)

export default JukeboxOutputCreate
