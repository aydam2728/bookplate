"use strict";

// The whole interface for the P1 walking skeleton: decide what to show, add a
// book, list the shelf. It is served as its own file rather than inline so the
// page can run under a Content-Security-Policy with no 'unsafe-inline'.

const el = (id) => document.getElementById(id);
const status = el("status");

function show(section) {
  for (const name of ["setup", "login", "shelf", "detail"]) {
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
// The copies the person has ticked, by copy identifier. A household
// reorganises a wall rather than a book, so the shelf can move many at once.
const selected = new Set();

function showSelection() {
  el("selection").hidden = selected.size === 0;
  el("selected-count").textContent = selected.size === 1
    ? "1 book chosen" : `${selected.size} books chosen`;
}

function render(books) {
  const list = el("books");
  list.replaceChildren();

  // "Nothing on the shelf yet" and "nothing matches" are different news, and
  // a person who has just typed a search needs the second one.
  const narrowed = filtering();
  const empty = el("empty");
  empty.hidden = books.length > 0;
  empty.textContent = narrowed ? "No book matches." : "Nothing on the shelf yet.";
  el("count").textContent = books.length
    ? `· ${books.length}${narrowed ? " found" : ""}` : "";

  // A line that is no longer listed cannot stay selected: a move must never
  // touch a book the person cannot see.
  const listed = new Set(books.map((book) => book.copyId));
  for (const copyId of selected) {
    if (!listed.has(copyId)) {
      selected.delete(copyId);
    }
  }

  for (const book of books) {
    const item = document.createElement("li");
    // The whole line opens the book's own page, so it carries both
    // identifiers: the copy for what sits on a shelf, the work for what
    // people think of it.
    item.className = "book";
    item.dataset.copyId = book.copyId;
    item.dataset.workId = book.workId;

    // Re-shelving a wall means picking many books, so every line carries a
    // box. The box keeps its own semantics: it is not inside the thing that
    // opens the book, or a screen reader would announce one control.
    const pick = document.createElement("input");
    pick.type = "checkbox";
    pick.className = "pick";
    pick.checked = selected.has(book.copyId);
    pick.setAttribute("aria-label", `Select ${book.title}`);
    item.append(pick);

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
    // The title is a real button, so a keyboard reaches the book by tabbing
    // and Enter, and the mouse gets the whole line through the handler below.
    const title = document.createElement("button");
    title.type = "button";
    title.className = "title open";
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
  showSelection();
}

// The shelf is whatever the two controls say it is, so every refresh reads
// them: adding a book while a search is on shows the search, not everything.
function filtering() {
  return el("q").value.trim() !== "" || el("filter-place").value !== "";
}

async function refresh() {
  const params = new URLSearchParams();
  const text = el("q").value.trim();
  if (text) {
    params.set("q", text);
  }
  const place = el("filter-place").value;
  if (place) {
    params.set("placeId", place);
  }
  const query = params.toString();

  const [books] = await Promise.all([
    call("GET", query ? `/api/books?${query}` : "/api/books"),
    refreshPlaces(),
  ]);
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
    // Clicking a place is how a person asks "what is in the living room",
    // which is the same question as a filtered shelf.
    item.dataset.placeId = place.id;
    item.tabIndex = 0;
    item.setAttribute("role", "button");
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

  // Every picker shows the same tree, indented with figure spaces so the
  // shape survives inside a select, where markup does not. What differs is
  // what the empty choice means, and the filter also offers "not filed".
  for (const [id, specials] of [
    ["place-parent", [["", "At the top"]]],
    ["book-place", [["", "Not filed yet"]]],
    ["detail-place", [["", "Not filed yet"]]],
    ["filter-place", [["", "Everywhere"], ["none", "Not filed yet"]]],
    ["move-place", [["", "Not filed yet"]]],
  ]) {
    const select = el(id);
    const chosen = select.value;
    select.replaceChildren();
    for (const [value, label] of specials) {
      const option = document.createElement("option");
      option.value = value;
      option.textContent = label;
      select.append(option);
    }
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

// A book's own page: what it is, where this copy sits, and what the reader
// thinks of it. Opened from the shelf, closed back to it.
let current = null;

// Ratings are stored in half stars, 1 to 10, because half stars are what
// people actually give. They are written the way people read them.
function stars(half) {
  if (!half) {
    return "Not rated";
  }
  const whole = Math.floor(half / 2);
  return `${whole || ""}${half % 2 ? "½" : ""} out of 5`;
}

function fillRatings() {
  const select = el("detail-rating");
  for (let half = 0; half <= 10; half++) {
    const option = document.createElement("option");
    option.value = String(half);
    option.textContent = stars(half);
    select.append(option);
  }
}
fillRatings();

// The household figure names nobody: it is an average and a head count, so
// one member's rating is never attributed to them.
function household(figures) {
  if (!figures.count) {
    return "Nobody here has rated it yet.";
  }
  const people = figures.count === 1 ? "1 person" : `${figures.count} people`;
  return `Household: ${(figures.average / 2).toFixed(1)} out of 5, from ${people}.`;
}

async function openDetail(copyId, workId) {
  const [book, opinion] = await Promise.all([
    call("GET", `/api/books/${encodeURIComponent(copyId)}`),
    call("GET", `/api/works/${encodeURIComponent(workId)}/rating`),
  ]);
  current = { copyId: book.copyId, workId: book.workId };

  const cover = el("detail-cover");
  cover.hidden = !book.coverId;
  if (book.coverId) {
    cover.src = `/api/covers/${encodeURIComponent(book.coverId)}`;
  } else {
    // An empty src would fetch the page itself, so the attribute goes.
    cover.removeAttribute("src");
  }

  el("detail-title").textContent = book.title;
  el("detail-meta").textContent = [book.authors, book.publisher, book.published,
    book.isbn13].filter(Boolean).join(" · ");
  el("detail-household").textContent = household(opinion.household);

  el("detail-place").value = book.placeId || "";
  el("detail-summary").value = book.summary || "";
  el("detail-notes").value = book.notes || "";

  el("detail-rating").value = String(opinion.mine.rating || 0);
  el("detail-status").value = opinion.mine.status || "";
  el("detail-visibility").value = opinion.mine.visibility || "private";
  el("detail-review").value = opinion.mine.review || "";

  show("detail");
  say("");
  el("back").focus();
}

// One listener on the list rather than one per line: the list is replaced on
// every refresh, and a per-line handler would be rebound each time.
function openFrom(item) {
  openDetail(item.dataset.copyId, item.dataset.workId).catch((error) => {
    say(error.message, "error");
  });
}

el("books").addEventListener("click", (event) => {
  const item = event.target.closest("li.book");
  if (!item) {
    return;
  }
  // The box is for choosing, the rest of the line for opening. Ticking a box
  // must not also open the book.
  if (event.target.closest("input.pick")) {
    if (event.target.checked) {
      selected.add(item.dataset.copyId);
    } else {
      selected.delete(item.dataset.copyId);
    }
    showSelection();
    return;
  }
  openFrom(item);
});

el("back").addEventListener("click", () => {
  current = null;
  show("shelf");
  say("");
});

submits("detail-form", async (values) => {
  if (!current) {
    return;
  }
  const book = await call("PATCH", `/api/books/${encodeURIComponent(current.copyId)}`, {
    summary: values.summary,
    notes: values.notes,
    placeId: values.placeId,
  });
  el("detail-place").value = book.placeId || "";
  say("Saved.");
  // The shelf and the place counters behind this page are now stale.
  await refresh();
});

submits("rating-form", async (values) => {
  if (!current) {
    return;
  }
  const opinion = await call("PUT", `/api/works/${encodeURIComponent(current.workId)}/rating`, {
    rating: Number(values.rating),
    review: values.review,
    status: values.status,
    visibility: values.visibility,
  });
  el("detail-household").textContent = household(opinion.household);
  say("Saved.");
});

// Searching is typed, not submitted: the list follows the box 200ms after the
// last keystroke, so a six-letter title costs one request and not six.
let pending = 0;

// Every refresh below is fire and forget, and a failure belongs in the status
// line rather than in a console nobody has open.
function reloading() {
  refresh().catch((error) => say(error.message, "error"));
}

el("q").addEventListener("input", () => {
  clearTimeout(pending);
  pending = setTimeout(reloading, 200);
});

el("filter-place").addEventListener("change", reloading);

el("clear-search").addEventListener("click", () => {
  el("q").value = "";
  el("filter-place").value = "";
  el("q").focus();
  reloading();
});

// Clicking a place filters the shelf to it and everything under it: a room
// includes its bookcases, because that is where the books actually are.
function filterTo(placeId) {
  el("filter-place").value = placeId;
  reloading();
}

el("places").addEventListener("click", (event) => {
  const item = event.target.closest("li.place");
  if (item) {
    filterTo(item.dataset.placeId);
  }
});

el("places").addEventListener("keydown", (event) => {
  if (event.key !== "Enter" && event.key !== " ") {
    return;
  }
  const item = event.target.closest("li.place");
  if (!item) {
    return;
  }
  // Space would scroll the page instead of filtering.
  event.preventDefault();
  filterTo(item.dataset.placeId);
});

// Moving the chosen books is one request, not one per book: the store files
// them in a single statement and answers with how many actually moved.
el("move").addEventListener("click", async () => {
  const button = el("move");
  button.disabled = true;
  try {
    const result = await call("POST", "/api/places/assign", {
      copyIds: [...selected],
      placeId: el("move-place").value,
    });
    selected.clear();
    say(result.moved === 1 ? "One book moved." : `${result.moved} books moved.`);
    await refresh();
  } catch (error) {
    say(error.message, "error");
  } finally {
    button.disabled = false;
  }
});

el("unselect").addEventListener("click", () => {
  selected.clear();
  for (const box of el("books").querySelectorAll("input.pick")) {
    box.checked = false;
  }
  showSelection();
});
