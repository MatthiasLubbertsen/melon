import { CalendarClock, CalendarRange, Coffee, Info, Layers, MessageSquareHeart, Users, type LucideIcon } from 'lucide-react'

export type Release = {
  /** Stable and unique: what a browser remembers as the last note it showed. */
  id: string
  /** "YYYY-MM-DD", the day it went live. */
  date: string
  icon: LucideIcon
  /** One short sentence, from the user's point of view. */
  nl: string
  en: string
}

/**
 * What changed in Melon, newest first. Add a line at the top when a change
 * is worth telling people about; it shows once, after they next open Melon.
 */
export const releases: Release[] = [
  {
    id: 'keep-day',
    date: '2026-11-19',
    icon: CalendarClock,
    nl: 'Wissel je van kind of account, dan blijf je op dezelfde dag staan.',
    en: 'Switching child or account now keeps you on the same day.',
  },
  {
    id: 'week-grid-tighter',
    date: '2026-11-19',
    icon: CalendarRange,
    nl: 'De weekweergave is compacter, zodat je hele week in één keer op je scherm past.',
    en: 'The week view is more compact, so your whole week fits on your screen at once.',
  },
  {
    id: 'parents',
    date: '2026-11-19',
    icon: Users,
    nl: 'Ouders zien nu het rooster van elk kind, met een eigen account per kind.',
    en: 'Parents now see each child’s schedule, with an account per child.',
  },
  {
    id: 'feedback',
    date: '2026-11-19',
    icon: MessageSquareHeart,
    nl: 'Mis je iets of werkt iets niet? Laat het weten met de knop rechtsonder.',
    en: 'Missing something or found a bug? Tell us with the button at the bottom right.',
  },
  {
    id: 'breaks',
    date: '2026-11-18',
    icon: Coffee,
    nl: 'Pauzes en tussenuren staan nu gewoon in je rooster.',
    en: 'Breaks and free periods now show up in your schedule.',
  },
  {
    id: 'about-page',
    date: '2026-11-18',
    icon: Info,
    nl: 'Op de pagina Over Melon lees je hoe Melon werkt en wat het met je gegevens doet.',
    en: 'The About Melon page explains how Melon works and what it does with your data.',
  },
  {
    id: 'overlap-cards',
    date: '2026-11-15',
    icon: Layers,
    nl: 'Lessen die tegelijk vallen staan samen in één kaart.',
    en: 'Lessons at the same time are grouped into one card.',
  },
]
