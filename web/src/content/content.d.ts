// Content files are imported as text: `import nl from './info.nl.md?raw'`.
declare module '*.md?raw' {
  const text: string
  export default text
}
