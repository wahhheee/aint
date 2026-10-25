import { cleanup, render, fireEvent, act } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { AssetRef } from '@/api/types'
import type { StageCue } from './scene'
import { StageAudio } from './StageAudio'

const settings = vi.hoisted(() => ({ audio: { master: 0.6, bgm: 1, sfx: 1, muted: false } }))
vi.mock('@/api/queries', () => ({ useSettings: () => ({ data: settings }) }))
const sounds: { play: ReturnType<typeof vi.fn>; pause: ReturnType<typeof vi.fn>; volume: number; loop: boolean }[] = []
beforeEach(() => {
  sounds.length = 0
  settings.audio.muted = false
  vi.useFakeTimers()
  vi.stubGlobal(
    'Audio',
    class {
      volume = 1
      loop = false
      play = vi.fn().mockResolvedValue(undefined)
      pause = vi.fn()
      constructor() {
        sounds.push(this)
      }
    },
  )
})
afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})
const assets: AssetRef[] = [{ id: 'rain', path: 'rain.wav', kind: 'audio', url: '/rain.wav', role: 'bgm', description: '' }]
it('播放只在交互后开始，历史音效不重播，同一实时音效只播一次，卸载停止音频', async () => {
  const cues: StageCue[] = [
    { kind: 'bgm', attrs: { src: 'rain', volume: '0.5' }, key: 'old:0' },
    { kind: 'sfx', attrs: { src: 'rain' }, key: 'old:1' },
    { kind: 'sfx', attrs: { src: 'rain' }, key: 'current:0' },
  ]
  const { rerender, unmount } = render(<StageAudio cues={cues} draftId="current" assets={assets} />)
  expect(sounds).toHaveLength(0)
  await act(async () => {
    fireEvent.pointerDown(window)
  })
  expect(sounds).toHaveLength(2)
  act(() => vi.advanceTimersByTime(1100))
  expect(sounds[0].volume).toBeCloseTo(0.3)
  rerender(<StageAudio cues={[...cues]} draftId="current" assets={assets} />)
  expect(sounds).toHaveLength(2)
  settings.audio.muted = true
  rerender(<StageAudio cues={[...cues]} draftId="current" assets={assets} />)
  expect(sounds[0].volume).toBe(0)
  expect(sounds[1].pause).toHaveBeenCalled()
  unmount()
  expect(sounds[0].pause).toHaveBeenCalled()
})
