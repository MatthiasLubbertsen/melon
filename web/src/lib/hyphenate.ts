/**
 * Soft hyphens for long Dutch words, so a narrow tile breaks "Handvaardigheid"
 * as "Handvaar-digheid" instead of "Handvaardighe-id". Browsers only hyphenate
 * Dutch where they ship a dictionary (not Chrome on Windows), so Melon marks
 * the syllable breaks itself; the browser still only breaks where it must.
 *
 * Rules (Dutch spelling, simplified):
 * - vowel groups like "aa", "ij", "oei" stay together, as do "ch" and "ng";
 * - one consonant between vowels starts the next syllable (be-we-gen);
 * - of more consonants, the next syllable takes the longest onset Dutch allows
 *   (kunst-zin-nig, maat-schap-pij), else only the last one (hand-vaar);
 * - an "s" after "ng" stays behind, as in compounds (be-we-gings-on-der-wijs);
 * - never leave fewer than three letters on either side.
 */

const SHY = '­'
const MIN_WORD = 8
const MIN_PART = 3

const vowelGroups = ['aai', 'ooi', 'oei', 'eeu', 'ieu', 'aa', 'ee', 'oo', 'uu', 'ie', 'oe', 'ou', 'au', 'ui', 'ei', 'ij', 'eu']
const onsets = new Set(['schr', 'sch', 'spr', 'str', 'spl', 'bl', 'br', 'dr', 'fl', 'fr', 'gl', 'gr', 'kl', 'kn', 'kr', 'pl', 'pr', 'tr', 'vl', 'vr', 'wr', 'zw', 'sl', 'sm', 'sn', 'sp', 'st', 'ch'])
const isVowel = (c: string) => 'aeiouyàáâäèéêëìíîïòóôöùúûü'.includes(c)

type Unit = { text: string; vowel: boolean }

/** Splits a lower-case word into vowel groups and consonant units. */
function units(word: string): Unit[] {
  const out: Unit[] = []
  let i = 0
  while (i < word.length) {
    const group = isVowel(word[i]) || word.startsWith('ij', i) ? vowelGroups.find((g) => word.startsWith(g, i)) ?? word[i] : null
    if (group && (isVowel(word[i]) || group === 'ij')) {
      out.push({ text: group, vowel: true })
      i += group.length
      continue
    }
    const pair = word.slice(i, i + 2)
    const text = pair === 'ch' || pair === 'ng' ? pair : word[i]
    out.push({ text, vowel: false })
    i += text.length
  }
  return out
}

/** Where a consonant cluster between two vowels splits: how many units go to the next syllable. */
function onsetLength(cluster: Unit[]): number {
  if (cluster.length <= 1) return cluster.length
  // A linking s after ng belongs to the first part of a compound.
  if (cluster.length >= 2 && cluster[cluster.length - 2].text === 'ng' && cluster[cluster.length - 1].text === 's') return 0
  for (let n = Math.min(3, cluster.length); n >= 2; n--) {
    if (onsets.has(cluster.slice(-n).map((u) => u.text).join(''))) return n
  }
  return 1
}

function hyphenateWord(word: string): string {
  if (word.length < MIN_WORD || !/^[\p{L}]+$/u.test(word)) return word
  const us = units(word.toLowerCase())
  const breaks: number[] = []
  let pos = 0
  let lastVowelEnd = -1
  let cluster: Unit[] = []
  let clusterStart = 0
  for (const u of us) {
    if (u.vowel) {
      if (lastVowelEnd >= 0) {
        const take = onsetLength(cluster)
        const at = clusterStart + cluster.slice(0, cluster.length - take).reduce((n, c) => n + c.text.length, 0)
        if (cluster.length > 0 || take === 0) breaks.push(at)
      }
      lastVowelEnd = pos + u.text.length
      cluster = []
      clusterStart = lastVowelEnd
    } else {
      if (cluster.length === 0) clusterStart = pos
      cluster.push(u)
    }
    pos += u.text.length
  }
  let out = ''
  let from = 0
  for (const at of breaks) {
    if (at < MIN_PART || word.length - at < MIN_PART) continue
    out += word.slice(from, at) + SHY
    from = at
  }
  return out + word.slice(from)
}

/** Adds soft hyphens to the long words of a Dutch text. Other languages are left to the browser. */
export function hyphenate(text: string, lang: string): string {
  if (lang !== 'nl') return text
  return text.replace(/[\p{L}]+/gu, hyphenateWord)
}
