import { expect, test } from 'vitest'
import { notify } from './notify'

test('a notify Step tells whether it sends the video of the Recording that started its Run', () => {
  expect(notify.summary({ title: 'Garden', message: 'Someone', video: true }, undefined as never)).toEqual(['Garden', 'Someone', '+ video'])
  expect(notify.summary({ title: 'Garden', message: '' }, undefined as never)).toEqual(['Garden'])
})
