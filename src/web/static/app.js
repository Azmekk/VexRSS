(() => {
  // ---------- clock ----------
  const clockTime = document.getElementById('clock-time');
  const clockDate = document.getElementById('clock-date');
  const timeFmt = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' });
  const dateFmt = new Intl.DateTimeFormat(undefined, { weekday: 'short', day: 'numeric', month: 'short' });
  function tick() {
    const now = new Date();
    if (clockTime) clockTime.textContent = timeFmt.format(now);
    if (clockDate) clockDate.textContent = dateFmt.format(now);
  }
  tick();
  setInterval(tick, 15_000);

  // When sources change anywhere (e.g. renamed via settings then back to feed),
  // ask the filter form to re-run so the grid refreshes.
  document.body.addEventListener('sources-changed', () => {
    document.body.dispatchEvent(new Event('refresh-cards'));
  });

  // ---------- custom <select> replacement ----------
  // Windows' native <option> popups ignore page styling and look jarring
  // against the dark UI. We hide each <select> and render a custom button +
  // listbox, while keeping the <select> in the DOM so form values and htmx's
  // "change from:select" trigger continue to work.
  function enhanceSelect(wrapper) {
    if (wrapper.dataset.csReady === '1') return;
    const select = wrapper.querySelector('select');
    if (!select) return;
    wrapper.dataset.csReady = '1';

    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'cs-btn';
    btn.setAttribute('aria-haspopup', 'listbox');
    btn.setAttribute('aria-expanded', 'false');

    const value = document.createElement('span');
    value.className = 'cs-value';

    const chev = document.createElement('span');
    chev.className = 'cs-chev';
    chev.innerHTML = '<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>';

    btn.append(value, chev);

    const list = document.createElement('ul');
    list.className = 'cs-list';
    list.setAttribute('role', 'listbox');
    list.hidden = true;

    function buildList() {
      list.innerHTML = '';
      Array.from(select.options).forEach((opt, i) => {
        const li = document.createElement('li');
        li.className = 'cs-opt';
        li.setAttribute('role', 'option');
        li.dataset.value = opt.value;
        li.textContent = opt.text;
        if (i === select.selectedIndex) {
          li.classList.add('cs-opt--selected');
          li.setAttribute('aria-selected', 'true');
        }
        li.addEventListener('mousedown', (e) => e.preventDefault()); // keep focus
        li.addEventListener('click', () => {
          if (select.value !== opt.value) {
            select.value = opt.value;
            select.dispatchEvent(new Event('change', { bubbles: true }));
          }
          close();
          btn.focus();
        });
        list.appendChild(li);
      });
    }

    function paintValue() {
      value.textContent = select.options[select.selectedIndex]?.text || '';
    }

    function isOpen() { return !list.hidden; }

    function open() {
      // Close any other open custom selects first.
      document.querySelectorAll('.select.cs-open').forEach(el => {
        if (el !== wrapper) el.dispatchEvent(new CustomEvent('cs:close'));
      });
      buildList();
      list.hidden = false;
      wrapper.classList.add('cs-open');
      btn.setAttribute('aria-expanded', 'true');
      // Put cursor on the currently-selected option
      const sel = list.querySelector('.cs-opt--selected');
      sel?.scrollIntoView({ block: 'nearest' });
    }
    function close() {
      list.hidden = true;
      wrapper.classList.remove('cs-open');
      btn.setAttribute('aria-expanded', 'false');
    }
    wrapper.addEventListener('cs:close', close);

    btn.addEventListener('click', (e) => {
      e.stopPropagation();
      e.preventDefault();
      isOpen() ? close() : open();
    });

    // Make the whole pill a hit target, not just the button text/chevron.
    // Clicks on the label span or the padding region also toggle.
    wrapper.addEventListener('click', (e) => {
      if (btn.contains(e.target)) return;   // button has its own handler
      if (list.contains(e.target)) return;  // option clicks handled per-row
      e.stopPropagation();
      isOpen() ? close() : open();
      btn.focus();
    });

    btn.addEventListener('keydown', (e) => {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        const dir = e.key === 'ArrowDown' ? 1 : -1;
        select.selectedIndex = Math.max(0, Math.min(select.options.length - 1, select.selectedIndex + dir));
        select.dispatchEvent(new Event('change', { bubbles: true }));
      } else if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        isOpen() ? close() : open();
      } else if (e.key === 'Escape' && isOpen()) {
        e.preventDefault();
        close();
      }
    });

    // Keep the custom UI in sync whenever the select's value changes
    // (e.g. via keyboard arrows or programmatic changes).
    select.addEventListener('change', () => { paintValue(); if (isOpen()) buildList(); });

    // Hide the native select visually but keep it focusable + form-submittable.
    select.classList.add('cs-native');

    wrapper.style.position = 'relative';
    wrapper.append(btn, list);
    paintValue();
  }

  function enhanceAllSelects(root = document) {
    root.querySelectorAll('.select').forEach(enhanceSelect);
  }
  enhanceAllSelects();
  // Re-enhance anything added by htmx swaps.
  document.body.addEventListener('htmx:afterSwap', (e) => enhanceAllSelects(e.target));

  // Any click outside an open custom-select closes them all.
  document.addEventListener('click', (e) => {
    document.querySelectorAll('.select.cs-open').forEach(w => {
      if (!w.contains(e.target)) w.dispatchEvent(new CustomEvent('cs:close'));
    });
  });

  // ---------- weather ----------
  const GEO_KEY = 'vexrss_geo_v1';
  const pill = document.getElementById('weather-pill');
  const pillIcon = pill?.querySelector('.weather-pill__icon');
  const pillTemp = pill?.querySelector('.weather-pill__temp');
  const modal = document.getElementById('weather-modal');
  const modalInput = document.getElementById('weather-input');
  const modalSave = document.getElementById('weather-save');
  const modalCancel = document.getElementById('weather-cancel');
  const modalError = document.getElementById('weather-error');
  const modalForm = document.getElementById('weather-form');

  const WCODE_EMOJI = [
    { codes: [0], emoji: '☀️' },
    { codes: [1, 2, 3], emoji: '⛅' },
    { codes: [45, 48], emoji: '🌫️' },
    { codes: [51, 53, 55, 56, 57], emoji: '🌦️' },
    { codes: [61, 63, 65, 66, 67, 80, 81, 82], emoji: '🌧️' },
    { codes: [71, 73, 75, 77, 85, 86], emoji: '❄️' },
    { codes: [95, 96, 99], emoji: '⛈️' },
  ];
  function weatherEmoji(code, isDay) {
    for (const g of WCODE_EMOJI) if (g.codes.includes(code)) {
      if (g.emoji === '☀️' && isDay === 0) return '🌙';
      if (g.emoji === '⛅' && isDay === 0) return '🌙';
      return g.emoji;
    }
    return '🌡️';
  }

  function loadGeo() {
    try { return JSON.parse(localStorage.getItem(GEO_KEY) || 'null'); } catch { return null; }
  }
  function saveGeo(v) { localStorage.setItem(GEO_KEY, JSON.stringify(v)); }

  async function fetchWeather(lat, lon, label) {
    const u = new URL('/api/weather', location.origin);
    u.searchParams.set('lat', lat);
    u.searchParams.set('lon', lon);
    if (label) u.searchParams.set('label', label);
    const res = await fetch(u);
    if (!res.ok) throw new Error('weather ' + res.status);
    return res.json();
  }

  function paintWeather(data) {
    if (!pill) return;
    pill.title = data.label ? `${data.label} — ${Math.round(data.temperature)}${data.units || '°C'}` : `${Math.round(data.temperature)}${data.units || '°C'}`;
    if (pillIcon) pillIcon.textContent = weatherEmoji(data.weatherCode, data.isDay);
    if (pillTemp) pillTemp.textContent = `${Math.round(data.temperature)}°`;
  }

  function paintUnknown(text) {
    if (!pillIcon || !pillTemp) return;
    pillIcon.textContent = '📍';
    pillTemp.textContent = text || 'Set';
  }

  async function refreshWeather() {
    const saved = loadGeo();
    if (!saved) {
      paintUnknown();
      return;
    }
    try {
      const data = await fetchWeather(saved.lat, saved.lon, saved.label);
      paintWeather(data);
    } catch {
      paintUnknown('--°');
    }
  }

  function openModal(err) {
    if (!modal) return;
    modalError.hidden = !err;
    if (err) modalError.textContent = err;
    modal.hidden = false;
    setTimeout(() => modalInput?.focus(), 30);
  }
  function closeModal() { if (modal) modal.hidden = true; }

  pill?.addEventListener('click', () => {
    if (!loadGeo() && 'geolocation' in navigator) {
      navigator.geolocation.getCurrentPosition(
        async (pos) => {
          const g = { lat: pos.coords.latitude, lon: pos.coords.longitude, label: '', ts: Date.now() };
          saveGeo(g);
          try { paintWeather(await fetchWeather(g.lat, g.lon)); } catch {}
        },
        () => openModal(),
        { timeout: 8000, maximumAge: 60 * 60 * 1000 }
      );
    } else {
      openModal();
    }
  });

  modalCancel?.addEventListener('click', closeModal);
  modalForm?.addEventListener('submit', async (e) => {
    e.preventDefault();
    const q = modalInput?.value?.trim();
    if (!q) return;
    modalSave.disabled = true;
    modalError.hidden = true;
    try {
      const res = await fetch('/api/geocode?q=' + encodeURIComponent(q));
      if (!res.ok) throw new Error('not found');
      const data = await res.json();
      const g = { lat: data.lat, lon: data.lon, label: data.label, ts: Date.now() };
      saveGeo(g);
      closeModal();
      try { paintWeather(await fetchWeather(g.lat, g.lon, g.label)); } catch {}
    } catch {
      modalError.hidden = false;
      modalError.textContent = 'Could not find that place.';
    } finally {
      modalSave.disabled = false;
    }
  });

  // On load: if geolocation is already cached, paint weather immediately.
  refreshWeather();
})();
