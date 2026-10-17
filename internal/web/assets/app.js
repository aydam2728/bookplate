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
    if (book.coverId) {
      const cover = document.createElement("img");
      cover.className = "cover";
      // Served from this instance, never hotlinked: the image was fetched,
      // re-encoded and cached when the book was added.
      cover.src = `/api/covers/${encodeURIComponent(book.coverId)}`;
      cover.alt = "";
      cover.loading = "lazy";
      item.append(cover);
    }
    const text = document.createElement("div");
    const title = document.createElement("div");
    title.className = "title";
    title.textContent = book.title;
    text.append(title);

    const bits = [book.authors, book.publisher, book.published, book.isbn13,
      book.placeName && `in ${book.placeName}`].filter(Boolean);
    if (bits.length) {
      const meta = document.createElement("div");
      meta.className = "meta";
      meta.textContent = bits.join(" · ");
      text.append(meta);
    }
    item.append(text);
    list.append(item);
  }
}

async function refresh() {
  const [books] = await Promise.all([call("GET", "/api/books"), refreshPlaces()]);
  render(books);
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
    const button = form.querySelector("button[type=submit]");
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

// Looking an ISBN up fills the form rather than adding the book, so a wrong
// record is corrected before anything is stored. A failure is never fatal:
// the fields stay editable and the book goes in by hand.
el("lookup").addEventListener("click", async () => {
  const isbnField = el("isbn");
  const isbn = isbnField.value.trim();
  if (!isbn) {
    isbnField.focus();
    say("Type or scan an ISBN first.");
    return;
  }

  const button = el("lookup");
  button.disabled = true;
  say("Looking it up…");
  try {
    const found = await call("GET", `/api/lookup?isbn=${encodeURIComponent(isbn)}`);
    const form = el("add-form");
    const fill = (name, value) => {
      if (value) form.querySelector(`input[name=${name}]`).value = value;
    };
    fill("title", found.subtitle ? `${found.title}: ${found.subtitle}` : found.title);
    fill("author", (found.authors || []).join(", "));
    fill("publisher", found.publisher);
    fill("published", found.published);
    el("coverUrl").value = found.coverUrl || "";
    say(`Found “${found.title}”. Check it, then add it.`);
  } catch (error) {
    say(error.message, "error");
  } finally {
    button.disabled = false;
  }
});

// Places: the tree of where things physically are. Indentation carries the
// hierarchy, and each line shows how many copies sit at or below it.
function renderPlaces(tree) {
  const list = el("places");
  list.replaceChildren();
  el("place-count").textContent = tree.length ? `· ${tree.length}` : "";

  for (const place of tree) {
    const item = document.createElement("li");
    item.className = "place";
    item.style.paddingLeft = `${14 + place.depth * 20}px`;

    const name = document.createElement("span");
    name.textContent = place.name;
    item.append(name);

    const count = document.createElement("span");
    count.className = "meta";
    count.textContent = place.copies === 1 ? "1 book" : `${place.copies} books`;
    item.append(count);

    list.append(item);
  }

  // Both pickers show the same tree, indented with figure spaces so the
  // shape survives inside a select, where markup does not.
  for (const [id, top] of [["place-parent", "At the top"], ["book-place", "Not filed yet"]]) {
    const select = el(id);
    const chosen = select.value;
    select.replaceChildren();
    const none = document.createElement("option");
    none.value = "";
    none.textContent = top;
    select.append(none);
    for (const place of tree) {
      const option = document.createElement("option");
      option.value = place.id;
      option.textContent = `${"  ".repeat(place.depth)}${place.name}`;
      select.append(option);
    }
    select.value = chosen;
  }
}

async function refreshPlaces() {
  renderPlaces(await call("GET", "/api/places"));
}

submits("place-form", async (values, form) => {
  await call("POST", "/api/places", { parentId: values.parentId || "", name: values.name });
  const parent = form.querySelector("select[name=parentId]").value;
  form.reset();
  // Keep the parent selected: adding five shelves to one bookcase is the
  // normal case, and resetting it every time would be a nuisance.
  form.querySelector("select[name=parentId]").value = parent;
  form.querySelector("input[name=name]").focus();
  await refreshPlaces();
  say("");
});
