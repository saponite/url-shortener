// Фронтенд url-shortener.
// Строится против реально существующих эндпоинтов (/create, /create_account).
// Логин/история/logout вынесены за чёткие «швы» (модули api + session) —
// когда на бэке появятся /login, /me, /logout, /links, фронт заработает без правок.

// ──────────────────────────────────────────────────────────────
// DOM-хелперы
// ──────────────────────────────────────────────────────────────
const $ = (id) => document.getElementById(id);
const on = (el, ev, fn) => el && el.addEventListener(ev, fn);

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
  const tone =
    kind === "success" ? "border-success/40 text-success"
    : kind === "error" ? "border-danger/40 text-danger"
    : "border-border text-fg";
  el.className =
    "pointer-events-auto animate-fade-in rounded-xl border bg-elevated px-4 py-2.5 " +
    "text-sm shadow-card " + tone;
  el.textContent = message;
  $("toaster").appendChild(el);
  setTimeout(() => {
    el.style.transition = "opacity .2s ease-out";
    el.style.opacity = "0";
    setTimeout(() => el.remove(), 200);
  }, 2600);
}

// ──────────────────────────────────────────────────────────────
// API-слой. Единая обработка ответа: сервер отдаёт ошибки то plain text
// (http.Error), то JSON (writeJSON) — извлекаем сообщение из обоих.
// ──────────────────────────────────────────────────────────────
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

class ApiError extends Error {
  constructor(message, status) {
    super(message);
    this.status = status;
  }
}

const api = {
  shorten: (originalUrl) =>
    request("/create", { method: "POST", body: { original_url: originalUrl } }),
  register: (user) =>
    request("/create_account", { method: "POST", body: user }),
  // ── Ждут бэкенда (контракт задокументирован в web/README-frontend.md) ──
  login: (creds) => request("/login", { method: "POST", body: creds }),
  logout: () => request("/logout", { method: "POST" }),
  me: () => request("/me"),
  links: () => request("/links"),
};

// ──────────────────────────────────────────────────────────────
// Сессия. Пока бэкенда логина нет — держим состояние в localStorage
// (как в исходном фронте). Точка замены на cookie-сессию — ровно здесь:
// заменить тело current()/на api.me() и refresh().
// ──────────────────────────────────────────────────────────────
const SESSION_KEY = "shortener_account_email";
const session = {
  current: () => localStorage.getItem(SESSION_KEY),
  set: (email) => localStorage.setItem(SESSION_KEY, email),
  clear: () => localStorage.removeItem(SESSION_KEY),
  isAuthed: () => !!localStorage.getItem(SESSION_KEY),
};

// ──────────────────────────────────────────────────────────────
// Валидация полей: показ/скрытие ошибки + aria-invalid
// ──────────────────────────────────────────────────────────────
function setError(input, errorEl, message) {
  if (message) {
    input.setAttribute("aria-invalid", "true");
    errorEl.textContent = message;
    errorEl.classList.remove("hidden");
  } else {
    input.removeAttribute("aria-invalid");
    errorEl.textContent = "";
    errorEl.classList.add("hidden");
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
    button.dataset.text = label ? label.textContent : "";
    if (label) label.textContent = "…";
  } else {
    button.disabled = false;
    if (label && button.dataset.text != null) label.textContent = button.dataset.text;
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
// Табы (ARIA): верхние (Сократить/Аккаунт) и вложенные (Вход/Регистрация)
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

  // ── Верхние табы ──
  const selectTop = wireTabs([
    { name: "shorten", btn: $("tabShorten"), panel: $("panelShorten") },
    { name: "auth", btn: $("tabAuth"), panel: $("panelAuth") },
  ]);

  // ── Вложенные табы аккаунта ──
  wireTabs([
    { name: "login", btn: $("tabLogin"), panel: $("loginForm") },
    { name: "register", btn: $("tabRegister"), panel: $("registerForm") },
  ]);

  // ── Состояние аккаунта ──
  function renderAccount() {
    const email = session.current();
    const bar = $("accountBar");
    if (email) {
      bar.classList.remove("hidden");
      bar.classList.add("flex");
      $("accountLabel").textContent = "Вы вошли как " + email;
      loadHistory();
    } else {
      bar.classList.add("hidden");
      bar.classList.remove("flex");
      $("historySection").classList.add("hidden");
    }
  }

  on($("logoutBtn"), "click", async () => {
    try { await api.logout(); } catch { /* бэкенда может не быть — не критично */ }
    session.clear();
    renderAccount();
    toast("Вы вышли", "info");
  });

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
    const box = $("result");
    const link = $("resultLink");
    link.href = shortUrl;
    link.textContent = shortUrl;
    box.classList.remove("hidden");
    box.setAttribute("tabindex", "-1");
    box.focus({ preventScroll: false }); // перевод фокуса на результат
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
      await api.login({ email: email.value.trim(), password: pass.value });
      session.set(email.value.trim());
      renderAccount();
      selectTop("shorten");
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
      session.set(email.value.trim());
      renderAccount();
      selectTop("shorten");
      toast("Аккаунт создан", "success");
    } catch (err) {
      showMsg($("registerResult"), err.message || "Не удалось зарегистрироваться");
    } finally {
      withLoading(btn, false);
    }
  });

  // ── История «Мои ссылки» ──
  async function loadHistory() {
    const section = $("historySection");
    const body = $("historyBody");
    section.classList.remove("hidden");
    body.innerHTML = skeletonRow();
    try {
      const links = await api.links();
      renderHistory(body, Array.isArray(links) ? links : []);
    } catch (err) {
      // Бэкенда истории ещё нет — деградируем тихо, без пугающих ошибок.
      if (err.status === 404 || err.status === 501) {
        section.classList.add("hidden");
      } else if (err.status === 401) {
        section.classList.add("hidden");
      } else {
        body.innerHTML =
          '<p class="text-[13px] text-muted">Не удалось загрузить историю</p>';
      }
    }
  }

  function renderHistory(body, links) {
    if (!links.length) {
      body.innerHTML =
        '<p class="rounded-xl border border-border bg-elevated p-3.5 text-[13px] text-muted">' +
        "Здесь появятся сокращённые вами ссылки</p>";
      return;
    }
    body.innerHTML = "";
    for (const l of links) {
      const row = document.createElement("div");
      row.className =
        "flex items-center gap-3 border-b border-border py-2.5 last:border-0";
      const short = l.short_url || l.short_code || "";
      row.innerHTML = `
        <div class="min-w-0 flex-1">
          <a href="${escapeAttr(short)}" target="_blank" rel="noopener"
             class="block truncate text-[14px] font-medium text-accent hover:underline">${escapeHtml(short)}</a>
          <p class="truncate text-[12px] text-faint">${escapeHtml(l.original_url || "")}</p>
        </div>
        <span class="shrink-0 text-[12px] text-muted">${(l.click_count ?? 0)} кл.</span>
        <button class="btn-ghost h-8 min-h-0 shrink-0 rounded-lg border border-border px-2.5 text-[12px]"
                type="button" data-copy="${escapeAttr(short)}">Копир.</button>`;
      body.appendChild(row);
    }
    body.querySelectorAll("[data-copy]").forEach((b) =>
      on(b, "click", async () => {
        const ok = await copyText(b.dataset.copy);
        toast(ok ? "Скопировано" : "Не удалось скопировать", ok ? "success" : "error");
      })
    );
  }

  function skeletonRow() {
    return '<div class="animate-pulse space-y-2">' +
      '<div class="h-4 w-2/3 rounded bg-border"></div>' +
      '<div class="h-4 w-1/2 rounded bg-border"></div></div>';
  }

  renderAccount();
}

// ── Мелкие утилиты ──
function showMsg(el, text) { el.textContent = text; el.classList.remove("hidden"); }
function hideMsg(el) { el.textContent = ""; el.classList.add("hidden"); }
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
