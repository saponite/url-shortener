// Фронтенд url-shortener.
// Строится против реально существующих эндпоинтов (/create, /create_account,
// /update_password). Логин/история/logout вынесены за швы (api + session) —
// когда на бэке появятся /login, /me, /logout, /links, фронт заработает без правок.

// ──────────────────────────────────────────────────────────────
// DOM-хелперы
// ──────────────────────────────────────────────────────────────
const $ = (id) => document.getElementById(id);
const on = (el, ev, fn) => el && el.addEventListener(ev, fn);
const show = (el) => el && el.classList.remove("is-hidden");
const hide = (el) => el && el.classList.add("is-hidden");
const isShown = (el) => el && !el.classList.contains("is-hidden");

// Разрешённые домены почты — зеркалим бэкенд (handler.go) для мгновенной валидации.
const ALLOWED_EMAIL_DOMAINS = new Set([
  "yandex.ru", "mail.ru", "internet.ru", "bk.ru", "list.ru", "inbox.ru", "ya.ru",
]);
const MIN_PASSWORD_LEN = 8;

// ──────────────────────────────────────────────────────────────
// Тосты (aria-live регион в разметке)
// ──────────────────────────────────────────────────────────────
function toast(message, kind = "info") {
  const el = document.createElement("div");
  el.className = "toast" + (kind === "success" ? " toast--success"
    : kind === "error" ? " toast--error" : "");
  el.textContent = message;
  $("toaster").appendChild(el);
  setTimeout(() => {
    el.style.opacity = "0";
    setTimeout(() => el.remove(), 200);
  }, 2600);
}

// ──────────────────────────────────────────────────────────────
// API-слой. Ошибки приходят то plain text (http.Error), то JSON (writeJSON) —
// извлекаем сообщение из обоих.
// ──────────────────────────────────────────────────────────────
class ApiError extends Error {
  constructor(message, status) {
    super(message);
    this.status = status;
  }
}

async function request(path, { method = "GET", body } = {}) {
  const res = await fetch(path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
    credentials: "same-origin", // чтобы будущая cookie-сессия ездила с запросами
  });
  const text = await res.text();
  let data = null;
  if (text) {
    try { data = JSON.parse(text); } catch { data = null; }
  }
  if (!res.ok) {
    const msg =
      (data && (data.error || data.message)) ||
      (typeof data === "string" && data) ||
      (text && text.trim()) ||
      `Ошибка ${res.status}`;
    throw new ApiError(msg, res.status);
  }
  return data;
}

const api = {
  shorten: (originalUrl) =>
    request("/create", { method: "POST", body: { original_url: originalUrl } }),
  register: (user) =>
    request("/create_account", { method: "POST", body: user }),
  updatePassword: (body) =>
    request("/update_password", { method: "PATCH", body }),
  // ── Ждут бэкенда (контракт задокументирован в web/README-frontend.md) ──
  login: (creds) => request("/login", { method: "POST", body: creds }),
  logout: () => request("/logout", { method: "POST" }),
  me: () => request("/me"),
  links: () => request("/links"),
};

// ──────────────────────────────────────────────────────────────
// Сессия. Пока бэкенда логина нет — держим { email, firstName, lastName }
// в localStorage. Точка замены на cookie-сессию — ровно здесь:
// current() → await api.me().
// ──────────────────────────────────────────────────────────────
const SESSION_KEY = "shortener_account";
const session = {
  current() {
    try { return JSON.parse(localStorage.getItem(SESSION_KEY)) || null; }
    catch { return null; }
  },
  set(user) { localStorage.setItem(SESSION_KEY, JSON.stringify(user)); },
  clear() { localStorage.removeItem(SESSION_KEY); },
  isAuthed() { return !!this.current(); },
};

function displayName(user) {
  return [user.firstName, user.lastName].filter(Boolean).join(" ") || user.email;
}

// ──────────────────────────────────────────────────────────────
// Валидация полей
// ──────────────────────────────────────────────────────────────
function setError(input, errorEl, message) {
  if (message) {
    input.setAttribute("aria-invalid", "true");
    errorEl.textContent = message;
    show(errorEl);
  } else {
    input.removeAttribute("aria-invalid");
    errorEl.textContent = "";
    hide(errorEl);
  }
  return !message;
}

function validateUrl(value) {
  const v = value.trim();
  if (!v) return "Введите ссылку";
  let host;
  try {
    const u = new URL(/^[a-z][a-z0-9+.-]*:\/\//i.test(v) ? v : "https://" + v);
    host = u.hostname;
  } catch {
    return "Похоже, это не ссылка";
  }
  if (!/^([a-z0-9-]+\.)+[a-z]{2,}$/i.test(host)) return "Некорректный домен";
  return null;
}

function validateEmail(value, { requireAllowedDomain }) {
  const v = value.trim();
  if (!v) return "Введите эл. почту";
  if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(v)) return "Некорректный адрес";
  if (requireAllowedDomain) {
    const domain = v.slice(v.lastIndexOf("@") + 1).toLowerCase();
    if (!ALLOWED_EMAIL_DOMAINS.has(domain)) {
      return "Домен не поддерживается — только почта рунета";
    }
  }
  return null;
}

// ──────────────────────────────────────────────────────────────
// Состояние загрузки кнопки (спиннер + защита от двойного сабмита)
// ──────────────────────────────────────────────────────────────
function withLoading(button, busy) {
  const label = button.querySelector("[data-label]");
  if (busy) {
    button.disabled = true;
    if (label) hide(label);
    if (!button.querySelector(".spinner")) {
      const s = document.createElement("span");
      s.className = "spinner";
      s.setAttribute("aria-hidden", "true");
      button.appendChild(s);
    }
  } else {
    button.disabled = false;
    const s = button.querySelector(".spinner");
    if (s) s.remove();
    if (label) show(label);
  }
}

// ──────────────────────────────────────────────────────────────
// Копирование в буфер (Clipboard API + фолбэк для http/старых браузеров)
// ──────────────────────────────────────────────────────────────
async function copyText(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch { /* упадём в фолбэк */ }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    ta.remove();
    return ok;
  } catch {
    return false;
  }
}

// ──────────────────────────────────────────────────────────────
// Табы (ARIA)
// ──────────────────────────────────────────────────────────────
function wireTabs(tabs) {
  const select = (name) => {
    for (const t of tabs) {
      const active = t.name === name;
      t.btn.setAttribute("aria-selected", active ? "true" : "false");
      t.panel.hidden = !active;
    }
  };
  for (const t of tabs) {
    on(t.btn, "click", () => {
      if (t.btn.disabled) return;
      select(t.name);
    });
  }
  return select;
}

// ──────────────────────────────────────────────────────────────
// Инициализация
// ──────────────────────────────────────────────────────────────
function init() {
  // ── Тема ──
  on($("themeToggle"), "click", () => {
    const dark = !document.documentElement.classList.contains("dark");
    document.documentElement.classList.toggle("dark", dark);
    try { localStorage.setItem("theme", dark ? "dark" : "light"); } catch {}
  });

  // ── Табы ──
  const selectTop = wireTabs([
    { name: "shorten", btn: $("tabShorten"), panel: $("panelShorten") },
    { name: "auth", btn: $("tabAuth"), panel: $("panelAuth") },
  ]);
  wireTabs([
    { name: "login", btn: $("tabLogin"), panel: $("loginForm") },
    { name: "register", btn: $("tabRegister"), panel: $("registerForm") },
  ]);

  // ── Меню аккаунта (дропдаун) ──
  const userMenu = $("userMenu");
  const menuTrigger = $("userMenuBtn");
  const menuDropdown = $("userMenuDropdown");

  function openUserMenu() {
    show(menuDropdown);
    menuTrigger.setAttribute("aria-expanded", "true");
  }
  function closeUserMenu() {
    hide(menuDropdown);
    menuTrigger.setAttribute("aria-expanded", "false");
  }
  on(menuTrigger, "click", (e) => {
    e.stopPropagation();
    isShown(menuDropdown) ? closeUserMenu() : openUserMenu();
  });
  document.addEventListener("click", (e) => {
    if (!userMenu.contains(e.target)) closeUserMenu();
  });

  // ── Модалка настроек ──
  const modal = $("settingsModal");
  function openModal() {
    show(modal);
    document.body.style.overflow = "hidden";
    $("newPassword").focus();
  }
  function closeModal() {
    hide(modal);
    document.body.style.overflow = "";
  }
  modal.querySelectorAll("[data-close-modal]").forEach((el) => on(el, "click", closeModal));

  // Esc закрывает и меню, и модалку
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      closeUserMenu();
      if (isShown(modal)) closeModal();
    }
  });

  on($("menuSettings"), "click", () => {
    closeUserMenu();
    openModal();
  });

  // ── Состояние аккаунта ──
  function renderAccount() {
    const user = session.current();
    if (user) {
      $("userMenuName").textContent = displayName(user);
      show(userMenu);
      hide($("tabs"));        // залогинен → раздел «Аккаунт» не нужен
      selectTop("shorten");
      loadHistory();
    } else {
      closeUserMenu();
      hide(userMenu);
      show($("tabs"));
      hide($("historySection"));
    }
  }

  async function logout() {
    try { await api.logout(); } catch { /* бэкенда может не быть — не критично */ }
    session.clear();
    renderAccount();
    toast("Вы вышли", "info");
  }
  on($("menuLogout"), "click", logout);

  // ── Форма: сокращение ──
  const urlInput = $("url");
  on(urlInput, "blur", () => {
    if (urlInput.value.trim()) setError(urlInput, $("urlError"), validateUrl(urlInput.value));
  });
  on(urlInput, "input", () => {
    if (urlInput.getAttribute("aria-invalid")) setError(urlInput, $("urlError"), null);
  });

  on($("shortenForm"), "submit", async (e) => {
    e.preventDefault();
    if (!setError(urlInput, $("urlError"), validateUrl(urlInput.value))) {
      urlInput.focus();
      return;
    }
    const btn = $("shortenSubmit");
    if (btn.disabled) return;
    withLoading(btn, true);
    try {
      const data = await api.shorten(urlInput.value.trim());
      showResult(data.short_url);
      if (session.isAuthed()) loadHistory();
    } catch (err) {
      setError(urlInput, $("urlError"), err.message);
      urlInput.focus();
    } finally {
      withLoading(btn, false);
    }
  });

  function showResult(shortUrl) {
    const link = $("resultLink");
    link.href = shortUrl;
    link.textContent = shortUrl;
    show($("result"));
    $("result").focus({ preventScroll: false });
    $("copyBtn").onclick = async () => {
      const ok = await copyText(shortUrl);
      toast(ok ? "Скопировано" : "Не удалось скопировать", ok ? "success" : "error");
    };
  }

  // ── Форма: вход ──
  on($("loginForm"), "submit", async (e) => {
    e.preventDefault();
    const email = $("loginEmail"), pass = $("loginPassword");
    const okEmail = setError(email, $("loginEmailError"),
      validateEmail(email.value, { requireAllowedDomain: false }));
    const okPass = setError(pass, $("loginPasswordError"),
      pass.value ? null : "Введите пароль");
    if (!okEmail || !okPass) return;

    const btn = $("loginSubmit");
    if (btn.disabled) return;
    withLoading(btn, true);
    hideMsg($("loginResult"));
    try {
      const res = await api.login({ email: email.value.trim(), password: pass.value });
      // Имя возьмём из ответа /me, когда он появится; пока — из ответа логина или email.
      session.set({
        email: email.value.trim(),
        firstName: res?.first_name || "",
        lastName: res?.last_name || "",
      });
      renderAccount();
      toast("С возвращением!", "success");
    } catch (err) {
      showMsg($("loginResult"),
        err.status === 404 ? "Вход пока недоступен" : (err.message || "Не удалось войти"));
    } finally {
      withLoading(btn, false);
    }
  });

  // ── Форма: регистрация ──
  on($("registerForm"), "submit", async (e) => {
    e.preventDefault();
    const first = $("regFirstName"), last = $("regLastName");
    const email = $("regEmail"), pass = $("regPassword");
    const ok = [
      setError(first, $("regFirstNameError"), first.value.trim() ? null : "Введите имя"),
      setError(last, $("regLastNameError"), last.value.trim() ? null : "Введите фамилию"),
      setError(email, $("regEmailError"),
        validateEmail(email.value, { requireAllowedDomain: true })),
      setError(pass, $("regPasswordError"),
        pass.value.length >= MIN_PASSWORD_LEN ? null
          : `Минимум ${MIN_PASSWORD_LEN} символов`),
    ].every(Boolean);
    if (!ok) return;

    const btn = $("registerSubmit");
    if (btn.disabled) return;
    withLoading(btn, true);
    hideMsg($("registerResult"));
    try {
      await api.register({
        first_name: first.value.trim(),
        last_name: last.value.trim(),
        email: email.value.trim(),
        password: pass.value,
      });
      session.set({
        email: email.value.trim(),
        firstName: first.value.trim(),
        lastName: last.value.trim(),
      });
      renderAccount();
      toast("Аккаунт создан", "success");
    } catch (err) {
      showMsg($("registerResult"), err.message || "Не удалось зарегистрироваться");
    } finally {
      withLoading(btn, false);
    }
  });

  // ── Настройки: смена пароля ──
  on($("updatePasswordForm"), "submit", async (e) => {
    e.preventDefault();
    const np = $("newPassword"), cp = $("confirmPassword");
    const okNp = setError(np, $("newPasswordError"),
      np.value.length >= MIN_PASSWORD_LEN ? null : `Минимум ${MIN_PASSWORD_LEN} символов`);
    const okCp = setError(cp, $("confirmPasswordError"),
      cp.value === np.value ? null : "Пароли не совпадают");
    if (!okNp || !okCp) return;

    const user = session.current();
    if (!user) { closeModal(); return; }

    const btn = $("updatePasswordSubmit");
    if (btn.disabled) return;
    withLoading(btn, true);
    hideMsg($("settingsResult"));
    try {
      await api.updatePassword({ email: user.email, password: np.value });
      np.value = "";
      cp.value = "";
      closeModal();
      toast("Пароль обновлён", "success");
    } catch (err) {
      showMsg($("settingsResult"),
        err.status === 404 ? "Смена пароля пока недоступна"
          : (err.message || "Не удалось обновить пароль"));
    } finally {
      withLoading(btn, false);
    }
  });

  // ── История «Мои ссылки» ──
  async function loadHistory() {
    const section = $("historySection");
    const body = $("historyBody");
    show(section);
    body.innerHTML = skeletonRows();
    try {
      const links = await api.links();
      renderHistory(body, Array.isArray(links) ? links : []);
    } catch (err) {
      // Бэкенда истории ещё нет / не авторизованы — деградируем тихо.
      if (err.status === 404 || err.status === 501 || err.status === 401) {
        hide(section);
      } else {
        body.innerHTML = '<p class="history-empty">Не удалось загрузить историю</p>';
      }
    }
  }

  function renderHistory(body, links) {
    if (!links.length) {
      body.innerHTML =
        '<p class="history-empty">Здесь появятся сокращённые вами ссылки</p>';
      return;
    }
    body.innerHTML = "";
    for (const l of links) {
      const short = l.short_url || l.short_code || "";
      const row = document.createElement("div");
      row.className = "history-item";
      row.innerHTML = `
        <div class="history-item__main">
          <a class="history-item__short" href="${escapeAttr(short)}" target="_blank" rel="noopener">${escapeHtml(short)}</a>
          <p class="history-item__orig">${escapeHtml(l.original_url || "")}</p>
        </div>
        <span class="history-item__meta">${(l.click_count ?? 0)} кл.</span>
        <button class="btn btn--outline" type="button" data-copy="${escapeAttr(short)}">Копир.</button>`;
      body.appendChild(row);
    }
    body.querySelectorAll("[data-copy]").forEach((b) =>
      on(b, "click", async () => {
        const ok = await copyText(b.dataset.copy);
        toast(ok ? "Скопировано" : "Не удалось скопировать", ok ? "success" : "error");
      })
    );
  }

  function skeletonRows() {
    return '<div class="skeleton">' +
      '<div class="skeleton__line"></div>' +
      '<div class="skeleton__line skeleton__line--2"></div>' +
      '<div class="skeleton__line skeleton__line--3"></div></div>';
  }

  renderAccount();
}

// ── Мелкие утилиты ──
function showMsg(el, text) { el.textContent = text; show(el); }
function hideMsg(el) { el.textContent = ""; hide(el); }
function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}
function escapeAttr(s) { return escapeHtml(s); }

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", init);
} else {
  init();
}
