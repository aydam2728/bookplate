"use strict";

// The whole interface for the P1 walking skeleton: decide what to show, add a
// book, list the shelf. It is served as its own file rather than inline so the
// page can run under a Content-Security-Policy with no 'unsafe-inline'.

const el = (id) => document.getElementById(id);
const status = el("status");

function show(section) {
  for (const name of ["setup", "login", "shelf"]) {
    el(name).hidden = name !== section;
  }
}

function say(message, kind) {
  status.textContent = message || "";
  if (kind) {
    status.dataset.kind = kind;
  } else {
    delete status.dataset.kind;
  }
}

async function call(method, path, body) {
  const options = { method, headers: { Accept: "application/json" } };
  if (body !== undefined) {
    options.headers["Content-Type"] = "application/json";
    options.body = JSON.stringify(body);
  }
  const response = await fetch(path, options);
  const text = await response.text();
  const payload = text ? JSON.parse(text) : null;
  if (!response.ok) {
    throw new Error((payload && payload.error) || "something went wrong");
  }
  return payload;
}

function fields(form) {
  const out = {};
  for (const [key, value] of new FormData(form).entries()) {
    out[key] = String(value).trim();
  }
  return out;
}

// textContent everywhere, never innerHTML: a title is typed by a person and
// one day will come from a public metadata API that strangers can edit.
function render(books) {
  const list = el("books");
  list.replaceChildren();
  el("empty").hidden = books.length > 0;
  el("count").textContent = books.length ? `· ${books.length}` : "";

  for (const book of books) {
    const item = document.createElement("li");
    const title = document.createElement("div");
    title.className = "title";
    title.textContent = book.title;
    item.append(title);

    const bits = [book.authors, book.publisher, book.published, book.isbn13].filter(Boolean);
    if (bits.length) {
      const meta = document.createElement("div");
      meta.className = "meta";
      meta.textContent = bits.join(" · ");
      item.append(meta);
    }
    list.append(item);
  }
}

async function refresh() {
  render(await call("GET", "/api/books"));
}

async function start() {
  try {
    const session = await call("GET", "/api/session");
    if (session.needsSetup) {
      show("setup");
      say("No account exists yet. Create the first one.");
      return;
    }
    if (!session.signedIn) {
      show("login");
      say("");
      return;
    }
    show("shelf");
    say("");
    await refresh();
  } catch (error) {
    say(error.message, "error");
  }
}

function submits(formId, handler) {
  el(formId).addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.target;
    const button = form.querySelector("button");
    button.disabled = true;
    try {
      await handler(fields(form), form);
    } catch (error) {
      say(error.message, "error");
    } finally {
      button.disabled = false;
    }
  });
}

submits("setup-form", async (values) => {
  await call("POST", "/api/setup", values);
  await start();
});

submits("login-form", async (values) => {
  await call("POST", "/api/login", values);
  await start();
});

submits("add-form", async (values, form) => {
  const book = await call("POST", "/api/books", values);
  form.reset();
  form.querySelector("input[name=title]").focus();
  say(`Added ${book.title}.`);
  await refresh();
});

el("signout").addEventListener("click", async () => {
  try {
    await call("POST", "/api/logout", {});
    await start();
  } catch (error) {
    say(error.message, "error");
  }
});

start();
