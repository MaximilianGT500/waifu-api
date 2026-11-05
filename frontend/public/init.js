(function () {
  try {
    var appTitle = (window.env && window.env.VITE_APP_TITLE) || 'Waifu API';
    document.title = appTitle;
    var ogTitle = document.querySelector('meta[property="og:title"]');
    if (ogTitle) ogTitle.setAttribute('content', appTitle);
    var twitterTitle = document.querySelector('meta[name="twitter:title"]');
    if (twitterTitle) twitterTitle.setAttribute('content', appTitle);
  } catch {}

  try {
    var savedTheme = localStorage.getItem('theme');
    var systemDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    var dark = savedTheme === 'dark' || ((!savedTheme || savedTheme === 'system') && systemDark);
    document.documentElement.classList.toggle('dark', dark);
    var themeColor = document.querySelector('meta[name="theme-color"]');
    var color = themeColor && themeColor.getAttribute(dark ? 'data-dark' : 'data-light');
    if (color) themeColor.setAttribute('content', color);
  } catch {}

  try {
    var accent = localStorage.getItem('accent');
    if (['pink', 'orange', 'green', 'blue', 'mono'].indexOf(accent) !== -1) {
      document.documentElement.setAttribute('data-accent', accent);
    }
  } catch {}

  try {
    var motion = null;
    try {
      motion = localStorage.getItem('motion');
    } catch {}
    var reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (motion === 'off' || (motion !== 'on' && reduceMotion)) {
      document.documentElement.setAttribute('data-motion', 'off');
    }
  } catch {}

  try {
    var env = window.env || {};
    var seen = {};
    [[env.VITE_API_URL, 'anonymous'], [env.VITE_CDN_ORIGIN, null]].forEach(function (entry) {
      if (!entry[0]) return;
      var url = new URL(entry[0], window.location.href);
      if (!/^https?:$/.test(url.protocol) || url.origin === window.location.origin) return;
      var key = url.origin + '|' + entry[1];
      if (seen[key]) return;
      seen[key] = true;
      var link = document.createElement('link');
      link.rel = 'preconnect';
      link.href = url.origin;
      if (entry[1]) link.crossOrigin = entry[1];
      document.head.appendChild(link);
    });
  } catch {}
})();
