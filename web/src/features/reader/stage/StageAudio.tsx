import { useEffect, useRef, useState } from 'react'
import { useSettings } from '@/api/queries'
import type { AssetRef } from '@/api/types'
import type { StageCue } from './scene'

/** 音频只播放作品素材；历史音效不重播，浏览器允许播放后才启动。 */
export function StageAudio({ cues, draftId, assets }: { cues: StageCue[]; draftId?: string; assets: AssetRef[] }) {
  const { data } = useSettings()
  const [unlocked, setUnlocked] = useState(false)
  const bgm = useRef<HTMLAudioElement | null>(null)
  const effects = useRef(new Set<HTMLAudioElement>())
  const seen = useRef(new Set<string>())
  const audio = data?.audio
  const last = cues.findLast((c) => c.kind === 'bgm')
  const level = audio?.muted ? 0 : (audio?.master ?? 0.6) * (audio?.bgm ?? 1) * cueVolume(last)
  const levelRef = useRef(level)
  const asset =
    last?.attrs.stop === 'true'
      ? undefined
      : assets.find((a) => a.kind === 'audio' && (a.id === last?.attrs.src || a.path === last?.attrs.src))
  useEffect(() => {
    if (unlocked) return
    const unlock = () => setUnlocked(true)
    window.addEventListener('pointerdown', unlock, { once: true })
    window.addEventListener('keydown', unlock, { once: true })
    return () => {
      window.removeEventListener('pointerdown', unlock)
      window.removeEventListener('keydown', unlock)
    }
  }, [unlocked])
  useEffect(() => {
    levelRef.current = level
    if (bgm.current) bgm.current.volume = level
  }, [level])
  useEffect(() => {
    if (!unlocked) return
    const old = bgm.current
    const next = asset ? new Audio(asset.url) : null
    bgm.current = next
    if (next) {
      next.loop = true
      next.volume = 0
      void next.play().catch(() => setUnlocked(false))
    }
    const start = performance.now()
    let raf = 0
    const fade = (now: number) => {
      const progress = Math.min(1, (now - start) / 1000)
      if (next) next.volume = levelRef.current * progress
      if (old) old.volume = levelRef.current * (1 - progress)
      if (progress < 1) raf = requestAnimationFrame(fade)
      else old?.pause()
    }
    raf = requestAnimationFrame(fade)
    return () => {
      cancelAnimationFrame(raf)
      old?.pause()
    }
  }, [asset?.url, unlocked]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!unlocked) return
    for (const cue of cues) {
      if (cue.kind !== 'sfx' || !draftId || !cue.key.startsWith(draftId + ':') || seen.current.has(cue.key)) continue
      seen.current.add(cue.key)
      const a = assets.find((a) => a.kind === 'audio' && (a.id === cue.attrs.src || a.path === cue.attrs.src))
      if (!a || audio?.muted) continue
      const sound = new Audio(a.url)
      sound.volume = (audio?.master ?? 0.6) * (audio?.sfx ?? 1) * cueVolume(cue)
      effects.current.add(sound)
      sound.onended = () => effects.current.delete(sound)
      void sound.play().catch(() => effects.current.delete(sound))
    }
  }, [cues, draftId, assets, audio, unlocked])
  useEffect(() => {
    const playing = effects.current
    if (audio?.muted) for (const sound of playing) sound.pause()
  }, [audio?.muted])
  useEffect(() => {
    const playing = effects.current
    return () => {
      bgm.current?.pause()
      for (const sound of playing) sound.pause()
      playing.clear()
    }
  }, [])
  return null
}

function cueVolume(cue?: StageCue) {
  const volume = Number(cue?.attrs.volume ?? 1)
  return Number.isFinite(volume) ? Math.min(1, Math.max(0, volume)) : 1
}
