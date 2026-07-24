// Desktop-shell helpers. The same frontend runs in the browser (web server) and
// inside the Wails desktop window; window.runtime exists only in the latter.
export const isDesktop = typeof window !== 'undefined' && !!window.runtime

// fetchPhrases prepends a browser-step note to a view's rotating loading
// phrases when running on the desktop (where a fetch opens a real Chrome and
// may show a captcha). On the web the base phrases are used unchanged.
export function fetchPhrases(base) {
  return isDesktop ? ['Відкриваю браузер — пройди «я не робот», якщо зʼявиться…', ...base] : base
}

// captchaHint explains the desktop browser step in one line.
export const captchaHint =
  'Відкриється вікно Chrome — це нормально. Якщо Cloudflare покаже «я не робот», клікни галочку. Перший запит ~20 секунд, далі — з кешу.'
