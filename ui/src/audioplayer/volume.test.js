import {
  VOLUME_EPSILON,
  devicePercentToPerceived,
  perceivedToDevicePercent,
  perceivedToElementVolume,
} from './volume'

describe('volume model', () => {
  // The store is the single authority: perceived 0..1. The <audio> element takes
  // the square, remote outputs take round(store*100). Changing any of these
  // breaks the slider, the keyboard shortcuts or the remote outputs.
  describe('perceivedToElementVolume', () => {
    it('squares the perceived value', () => {
      expect(perceivedToElementVolume(0.5)).toBeCloseTo(0.25)
      expect(perceivedToElementVolume(1)).toBe(1)
      expect(perceivedToElementVolume(0)).toBe(0)
    })

    it('clamps out-of-range input instead of throwing', () => {
      expect(perceivedToElementVolume(2)).toBe(1)
      expect(perceivedToElementVolume(-1)).toBe(0)
      expect(perceivedToElementVolume(undefined)).toBe(0)
    })
  })

  describe('devicePercentToPerceived', () => {
    it('converts a valid percentage', () => {
      expect(devicePercentToPerceived(65)).toBeCloseTo(0.65)
      expect(devicePercentToPerceived(100)).toBe(1)
    })

    it('ignores 0 so a mute is never adopted from a driver', () => {
      // Drivers report 0 until the device answers its first volume query;
      // adopting it is the historical "volume is 0% after a refresh" bug.
      expect(devicePercentToPerceived(0)).toBeNull()
    })

    it('ignores nonsense values', () => {
      expect(devicePercentToPerceived(-1)).toBeNull()
      expect(devicePercentToPerceived(101)).toBeNull()
      expect(devicePercentToPerceived(NaN)).toBeNull()
      expect(devicePercentToPerceived('80')).toBeNull()
      expect(devicePercentToPerceived(undefined)).toBeNull()
    })
  })

  describe('perceivedToDevicePercent', () => {
    it('rounds to the 0-100 integer the drivers expect', () => {
      expect(perceivedToDevicePercent(0.655)).toBe(66)
      expect(perceivedToDevicePercent(0)).toBe(0)
      expect(perceivedToDevicePercent(1)).toBe(100)
      expect(perceivedToDevicePercent(1.4)).toBe(100)
      expect(perceivedToDevicePercent(undefined)).toBe(0)
    })
  })

  it('keeps a tolerance so the element is not rewritten on every ping-pong', () => {
    expect(VOLUME_EPSILON).toBeGreaterThan(0)
    expect(VOLUME_EPSILON).toBeLessThan(0.01)
  })
})
