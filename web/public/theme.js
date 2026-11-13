// Applies the saved theme before first paint, so there is no flash.
(function () {
  try {
    var t = localStorage.getItem('theme') || 'system'
    var dark = t === 'dark' || (t === 'system' && matchMedia('(prefers-color-scheme: dark)').matches)
    document.documentElement.classList.toggle('dark', dark)
    var l = localStorage.getItem('melon.lang')
    if (l === 'en') document.documentElement.lang = 'en'
  } catch {
    /* storage blocked: defaults apply */
  }
})()
