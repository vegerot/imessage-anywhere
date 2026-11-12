// The page's behaviour. `go generate` type-checks this file and compiles it to app.js (tsc
// with erasableSyntaxOnly, so compiling only strips the types); the Proxy embeds app.js.

// ── The Proxy's JSON ──────────────────────────────────────────────────────────────────

interface Conversation {
	id: number;
	name: string;
	service: string; // "iMessage", "SMS", or "RCS"
	isGroup: boolean;
	unread: number;
	lastMessageAt: string;
}

interface Reaction {
	emoji: string;
	from: string;
	fromMe?: boolean;
}

type Delivery = "sending" | "unconfirmed" | "failed";

interface Message {
	id: number; // database row ID; negative for a bubble I'm still sending
	guid: string;
	conversationId: number;
	fromMe: boolean;
	sender: string;
	senderName?: string;
	text: string;
	createdAt: string;
	edited?: boolean;
	reactions?: Reaction[];
	placeholder?: string; // what the page can't show yet (attachments, polls, …)
	reactsTo?: string; // a live tapback: the GUID of the Message it reacts to
	tapback?: string;
	tapbackRemoved?: boolean;
	delivery?: Delivery; // only on my own bubbles while they're unconfirmed
}

interface SendResult {
	status: "confirmed" | "unconfirmed";
	id?: number;
}

// The one place JSON enters: the Proxy is ours, so its shapes are trusted, not validated.
function fromJSON<T>(text: string): T {
	return JSON.parse(text) as T;
}

async function getJSON<T>(url: string): Promise<T> {
	const response = await fetch(url);
	if (!response.ok) throw new Error(`${url}: ${response.status} ${await response.text()}`);
	return fromJSON<T>(await response.text());
}

// ── The page's own elements ───────────────────────────────────────────────────────────

// Elements of index.html's own markup: missing one is a programming error, so it throws.
function find<T extends Element>(root: ParentNode, selector: string, type: { new (): T; prototype: T }): T {
	const element = root.querySelector(selector);
	if (!(element instanceof type)) throw new Error(`index.html has no ${type.name} matching ${selector}`);
	return element;
}

function fromTemplate<T extends Element>(template: HTMLTemplateElement, type: { new (): T; prototype: T }): T {
	const element = template.content.firstElementChild?.cloneNode(true);
	if (!(element instanceof type)) throw new Error(`template #${template.id} doesn't start with a ${type.name}`);
	return element;
}

const list = find(document, "#conversations", HTMLUListElement);
const main = find(document, "main", HTMLElement);
const status = find(document, "#status", HTMLParagraphElement);
const connection = find(document, "#connection", HTMLParagraphElement);
const rowTemplate = find(document, "#conversation-row", HTMLTemplateElement);
const viewTemplate = find(document, "#conversation-view", HTMLTemplateElement);
const personGlyph = find(document, "#person-glyph", HTMLTemplateElement);

// ── Dates, Messages.app style ─────────────────────────────────────────────────────────

const dayFormat = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
const timeOfDay = (date: Date) => date.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
const capitalized = (text: string) => text.charAt(0).toUpperCase() + text.slice(1);

function daysAgo(date: Date): number {
	const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
	return Math.round((startOfDay(new Date()) - startOfDay(date)) / 86_400_000);
}

// The list: a time today, "Yesterday", a weekday within the week, else a date.
function formatWhen(date: Date): string {
	const days = daysAgo(date);
	if (days === 0) return timeOfDay(date);
	if (days === 1) return capitalized(dayFormat.format(-1, "day"));
	if (days < 7) return date.toLocaleDateString([], { weekday: "long" });
	return date.toLocaleDateString([], { year: "2-digit", month: "numeric", day: "numeric" });
}

// Time dividers: "Today 12:26 AM", "Yesterday 8:13 AM", "Tuesday 3:00 PM", "Nov 2, 2026 3:00 PM".
function dividerParts(date: Date): [day: string, time: string] {
	const days = daysAgo(date);
	if (days < 2) return [capitalized(dayFormat.format(-days, "day")), timeOfDay(date)];
	if (days < 7) return [date.toLocaleDateString([], { weekday: "long" }), timeOfDay(date)];
	return [date.toLocaleDateString([], { month: "short", day: "numeric", year: "numeric" }), timeOfDay(date)];
}

// ── The Conversation list ─────────────────────────────────────────────────────────────

const conversations = new Map<number, Conversation>();
const rows = new Map<number, HTMLLIElement>();

// Initials for a name; a person glyph for a raw number, email, or short code.
function fillAvatar(avatar: HTMLElement, name: string): void {
	const words = name.split(/\s+/).filter((word) => /^\p{L}/u.test(word));
	if (words.length === 0) {
		avatar.append(personGlyph.content.cloneNode(true));
		return;
	}
	avatar.textContent = words
		.slice(0, 2)
		.map((word) => word.charAt(0).toUpperCase())
		.join("");
}

function renderRow(conversation: Conversation): HTMLLIElement {
	const row = fromTemplate(rowTemplate, HTMLLIElement);
	find(row, "a", HTMLAnchorElement).href = `/conversations/${conversation.id}`;
	fillAvatar(find(row, ".avatar", HTMLElement), conversation.name);
	find(row, ".name", HTMLElement).textContent = conversation.name;
	showWhen(row, conversation.lastMessageAt);
	row.toggleAttribute("data-unread", conversation.unread > 0);
	return row;
}

function showWhen(row: HTMLLIElement, at: string): void {
	const when = find(row, ".when", HTMLTimeElement);
	when.dateTime = at;
	when.textContent = formatWhen(new Date(at));
}

const preview = (row: HTMLLIElement) => find(row, ".preview", HTMLElement);

function previewText(message: Message): string {
	if (message.reactsTo) return `${message.fromMe ? "You" : message.senderName} reacted ${message.tapback}`;
	return message.text || message.placeholder || "";
}

async function loadConversations(): Promise<void> {
	const fresh = await getJSON<Conversation[]>("/api/conversations");
	conversations.clear();
	rows.clear();
	for (const conversation of fresh) {
		conversations.set(conversation.id, conversation);
		rows.set(conversation.id, renderRow(conversation));
	}
	list.replaceChildren(...rows.values());
	if (openID !== null) rows.get(openID)?.querySelector("a")?.setAttribute("aria-current", "page");
}

// The open Conversation's history goes first: imsg's reads contend, and with previews in
// flight a 0.5 s history read took 2.75 s (measured 2026-11-12).
let conversationLoading: Promise<unknown> = Promise.resolve();

// Previews need one request per Conversation, so they fill in after the list shows. One
// fill runs at a time; a fill requested meanwhile runs after it (e.g. after a list reload).
let filling: Promise<void> | null = null;
let fillAgain = false;

function fillPreviews(): void {
	if (filling) {
		fillAgain = true;
		return;
	}
	filling = fillOnce().finally(() => {
		filling = null;
		if (fillAgain) {
			fillAgain = false;
			fillPreviews();
		}
	});
}

async function fillOnce(): Promise<void> {
	const queue = [...rows.keys()];
	const worker = async () => {
		for (let id = queue.shift(); id !== undefined; id = queue.shift()) {
			await conversationLoading;
			const row = rows.get(id);
			if (!row || preview(row).textContent) continue; // gone, or filled live
			const [last] = await getJSON<Message[]>(`/api/conversations/${id}/messages?limit=1`).catch(() => []);
			if (last) noteSeen(last);
			preview(row).textContent = last ? previewText(last) : "";
		}
	};
	// Two workers leave two of imsg's four read lanes free, so opening a Conversation never waits behind previews.
	await Promise.all([worker(), worker()]);
}

// ── The open Conversation ─────────────────────────────────────────────────────────────

let openID: number | null = null;
let openMessages: Message[] = [];
let lastEventID = 0; // the newest Message row ID seen anywhere: the live stream resumes after it

function noteSeen(message: Message): void {
	lastEventID = Math.max(lastEventID, message.id);
}

// Text with clickable links; everything else stays plain text (never parsed as HTML).
function linkified(text: string): DocumentFragment {
	const fragment = document.createDocumentFragment();
	let rest = 0;
	for (const match of text.matchAll(/https?:\/\/[^\s<>"]+[^\s<>".,;:!?)\]]/g)) {
		fragment.append(text.slice(rest, match.index));
		const link = document.createElement("a");
		link.href = match[0];
		link.target = "_blank";
		link.rel = "noopener noreferrer";
		link.textContent = match[0];
		fragment.append(link);
		rest = match.index + match[0].length;
	}
	fragment.append(text.slice(rest));
	return fragment;
}

function item(className: string, ...content: (Node | string)[]): HTMLLIElement {
	const li = document.createElement("li");
	li.className = className;
	li.append(...content);
	return li;
}

function small(className: string, text: string): HTMLElement {
	const label = document.createElement("small");
	label.className = className;
	label.textContent = text;
	return label;
}

const dividerGap = 60 * 60 * 1000; // a new time divider after an hour of silence

function renderMessages(conversation: Conversation, messages: Message[]): HTMLLIElement[] {
	const items: HTMLLIElement[] = [];
	let previous: Message | undefined;
	for (const message of messages) {
		const at = new Date(message.createdAt);
		const newDivider = !previous || at.getTime() - new Date(previous.createdAt).getTime() > dividerGap;
		if (newDivider) {
			const [day, time] = dividerParts(at);
			const bold = document.createElement("b");
			bold.textContent = day;
			items.push(item("divider", bold, ` ${time}`));
		}
		const newSender = newDivider || previous?.fromMe !== message.fromMe || previous?.sender !== message.sender;
		if (conversation.isGroup && !message.fromMe && newSender) items.push(item("sender", message.senderName ?? ""));
		items.push(renderMessage(conversation, message));
		previous = message;
	}
	return items;
}

// What the page shows under a bubble I'm sending. "unconfirmed" may have been delivered,
// so it's never resent automatically.
const deliveryLabels: Record<Delivery, string> = {
	sending: "Sending…",
	unconfirmed: "Not confirmed. Check Messages before resending.",
	failed: "Not sent.",
};

function renderMessage(conversation: Conversation, message: Message): HTMLLIElement {
	const bubble = document.createElement("p");
	bubble.className = "bubble";
	if (message.text) bubble.append(linkified(message.text));
	if (message.placeholder) {
		const placeholder = document.createElement("span");
		placeholder.className = "placeholder";
		placeholder.textContent = message.placeholder;
		bubble.append(placeholder);
	}
	if (message.reactions?.length) {
		const tapbacks = document.createElement("span");
		tapbacks.className = "tapbacks";
		tapbacks.textContent = message.reactions.map((r) => r.emoji).join("");
		tapbacks.title = message.reactions.map((r) => `${r.from}: ${r.emoji}`).join("\n");
		bubble.append(tapbacks);
	}

	const li = item("message", bubble);
	li.classList.toggle("from-me", message.fromMe);
	li.classList.toggle("imessage", conversation.service === "iMessage");
	li.dataset["id"] = String(message.id);
	if (message.delivery) {
		const label = small("delivery", deliveryLabels[message.delivery]);
		label.classList.toggle("warning", message.delivery !== "sending");
		li.append(label);
	}
	if (message.edited) li.append(small("edited", "Edited"));
	return li;
}

function showMessages(conversation: Conversation, { scrollToEnd }: { scrollToEnd: boolean }): void {
	const messages = find(main, "#messages", HTMLOListElement);
	const nearEnd = messages.scrollHeight - messages.scrollTop - messages.clientHeight < 80;
	messages.replaceChildren(...renderMessages(conversation, openMessages));
	if (scrollToEnd || nearEnd) messages.scrollTop = messages.scrollHeight;
}

// Shows a Conversation's frame (header, composer, selected row) synchronously, so a View
// Transition can animate it without waiting for the network; or the empty state.
function showConversation(id: number | null): void {
	openID = id;
	openMessages = [];
	for (const [rowID, row] of rows) {
		const link = find(row, "a", HTMLAnchorElement);
		if (rowID === id) link.setAttribute("aria-current", "page");
		else link.removeAttribute("aria-current");
	}
	const conversation = id === null ? undefined : conversations.get(id);
	if (!conversation) {
		main.removeAttribute("data-open");
		main.replaceChildren(status);
		status.textContent = id === null ? "Select a Conversation" : "This Conversation isn’t among the newest 100.";
		return;
	}
	main.setAttribute("data-open", "");
	main.replaceChildren(viewTemplate.content.cloneNode(true));
	find(main, "h2", HTMLHeadingElement).textContent = conversation.name;
	main.classList.toggle("imessage", conversation.service === "iMessage");
	setUpComposer(conversation);
}

async function loadConversation(id: number | null): Promise<void> {
	const conversation = id === null ? undefined : conversations.get(id);
	if (!conversation) return;
	const loading = getJSON<Message[]>(`/api/conversations/${conversation.id}/messages`);
	conversationLoading = loading.catch(() => {});
	const messages = await loading;
	if (openID !== conversation.id) return; // the user opened another Conversation meanwhile
	messages.forEach(noteSeen);
	openMessages = messages;
	showMessages(conversation, { scrollToEnd: true });
}

// ── Sending ───────────────────────────────────────────────────────────────────────────

const sendsOnEnter = matchMedia("(pointer: fine)").matches; // desktop, like Messages.app
let pendingCount = 0;

function setUpComposer(conversation: Conversation): void {
	const form = find(main, ".composer", HTMLFormElement);
	const textarea = find(form, "textarea", HTMLTextAreaElement);
	textarea.placeholder = conversation.service === "iMessage" ? "iMessage" : `Text Message • ${conversation.service}`;
	textarea.addEventListener("keydown", (event) => {
		if (sendsOnEnter && event.key === "Enter" && !event.shiftKey && !event.isComposing) {
			event.preventDefault();
			form.requestSubmit();
		}
	});
	form.addEventListener("submit", async (event) => {
		event.preventDefault();
		const text = textarea.value;
		textarea.value = "";
		const pending: Message = {
			id: -++pendingCount,
			guid: "",
			conversationId: conversation.id,
			fromMe: true,
			sender: "",
			text,
			createdAt: new Date().toISOString(),
			delivery: "sending",
		};
		openMessages.push(pending);
		showMessages(conversation, { scrollToEnd: true });
		const delivery = await send(conversation.id, text, pending);
		if (delivery) pending.delivery = delivery;
		else delete pending.delivery; // confirmed: no label
		if (openID === conversation.id) showMessages(conversation, { scrollToEnd: false });
	});
}

// Returns the pending bubble's delivery state once the Proxy answers (undefined: confirmed).
async function send(conversationID: number, text: string, pending: Message): Promise<Delivery | undefined> {
	try {
		const response = await fetch(`/api/conversations/${conversationID}/messages`, {
			method: "POST",
			body: new URLSearchParams({ text }),
		});
		if (!response.ok) return "failed";
		const result = fromJSON<SendResult>(await response.text());
		if (result.status !== "confirmed" || result.id === undefined) return "unconfirmed";
		const confirmedID = result.id;
		// The live stream may already have delivered this row; then drop the pending copy.
		if (openMessages.some((m) => m.id === confirmedID)) {
			openMessages = openMessages.filter((m) => m !== pending);
		} else {
			pending.id = confirmedID;
		}
		return undefined;
	} catch {
		return "unconfirmed"; // the request may have reached the Proxy before the network failed
	}
}

// ── Live updates ──────────────────────────────────────────────────────────────────────

let reloading: Promise<void> | null = null;

// A new or changed Message arrived live: update its row, and the open Conversation.
// isNew is false for edits and unsends, which reuse an older row ID.
async function applyLiveMessage(message: Message, isNew: boolean): Promise<void> {
	const id = message.conversationId;
	if (!conversations.has(id)) {
		// Outside the list: only a new row can bring it into the newest 100. One reload at a time.
		if (!isNew) return;
		reloading ??= loadConversations().finally(() => {
			reloading = null;
			fillPreviews();
		});
		await reloading;
	}
	const conversation = conversations.get(id);
	const row = rows.get(id);
	if (!conversation || !row) return;
	if (isNew && message.createdAt >= conversation.lastMessageAt) {
		conversation.lastMessageAt = message.createdAt;
		showWhen(row, message.createdAt);
		preview(row).textContent = previewText(message);
		list.prepend(row);
		if (!message.fromMe && openID !== id) row.toggleAttribute("data-unread", true);
	}
	if (openID !== id) return;
	if (message.reactsTo) {
		applyTapback(conversation, message);
		return;
	}
	let index = openMessages.findIndex((m) => m.id === message.id);
	if (index < 0 && message.fromMe) {
		// My own send arriving live, before the POST answered or after it came back
		// unconfirmed (it was delivered after all): it takes over the pending bubble.
		index = openMessages.findIndex((m) => (m.delivery === "sending" || m.delivery === "unconfirmed") && m.text.trim() === message.text);
	}
	if (index >= 0) openMessages[index] = message; // an edit, a replay, or a send confirmed live
	else openMessages.push(message);
	showMessages(conversation, { scrollToEnd: message.fromMe });
}

// A tapback is its own row pointing at the Message it reacts to (by GUID).
function applyTapback(conversation: Conversation, tapback: Message): void {
	const target = openMessages.find((m) => m.guid === tapback.reactsTo);
	if (!target || !tapback.tapback) return; // reacting to a Message older than the 100 shown
	const emoji = tapback.tapback;
	const from = tapback.fromMe ? "You" : (tapback.senderName ?? "");
	target.reactions = (target.reactions ?? []).filter((r) => !(r.from === from && r.emoji === emoji));
	if (!tapback.tapbackRemoved) target.reactions.push({ emoji, from, fromMe: tapback.fromMe });
	showMessages(conversation, { scrollToEnd: false });
}

const quietLimit = 60_000; // the Proxy sends a heartbeat every 25 s
let source: EventSource | undefined;
let watchdog: number | undefined;

// Subscribe before loading, so a Message arriving meanwhile isn't lost; hold it until the
// list exists.
let listReady = false;
const early: [Message, boolean][] = [];

function setOnline(online: boolean): void {
	connection.hidden = online;
}

// EventSource reconnects by itself after errors, sending Last-Event-ID. The watchdog
// covers the case it can't see: a connection that died silently (the Host Mac slept).
function connect(): void {
	source?.close();
	source = new EventSource(lastEventID ? `/api/events?since=${lastEventID}` : "/api/events");
	source.addEventListener("open", () => setOnline(true));
	source.addEventListener("error", () => setOnline(false));
	source.addEventListener("heartbeat", resetWatchdog);
	source.addEventListener("message", (event: MessageEvent<string>) => {
		resetWatchdog();
		const message = fromJSON<Message>(event.data);
		const isNew = message.id > lastEventID;
		noteSeen(message);
		if (listReady) applyLiveMessage(message, isNew);
		else early.push([message, isNew]);
	});
	resetWatchdog();
}

function resetWatchdog(): void {
	clearTimeout(watchdog);
	watchdog = setTimeout(() => {
		setOnline(false);
		connect();
	}, quietLimit);
}

// ── Navigation: clean paths, animated like Messages ───────────────────────────────────

const reduceMotion = matchMedia("(prefers-reduced-motion: reduce)");
const conversationPath = /^\/conversations\/(\d+)$/;

function conversationIDFromPath(): number | null {
	const match = location.pathname.match(conversationPath);
	return match ? Number(match[1]) : null;
}

// Shows whatever the path names. On navigation it animates like Messages: at phone width,
// an iOS push (or pop) between the list and the Conversation; a quick fade on desktop.
function route({ animate }: { animate: boolean }): void {
	const id = conversationIDFromPath();
	const previousID = openID;
	if (!animate || reduceMotion.matches) {
		showConversation(id);
		afterRoute(id, previousID);
		return;
	}
	document.documentElement.dataset["direction"] = id === null ? "back" : "forward";
	const transition = document.startViewTransition(() => showConversation(id));
	transition.finished.finally(() => afterRoute(id, previousID));
}

// Focus follows the navigation (a View Transition doesn't move it), then Messages load.
function afterRoute(id: number | null, previousID: number | null): void {
	if (id !== null) {
		main.querySelector("h2")?.focus({ preventScroll: true });
	} else if (previousID !== null) {
		rows.get(previousID)?.querySelector("a")?.focus({ preventScroll: true });
	}
	loadConversation(id).catch((error: unknown) => {
		main.replaceChildren(status);
		status.textContent = `Couldn’t load this Conversation: ${error instanceof Error ? error.message : error}. Reload to try again.`;
	});
}

// Plain clicks on Conversation links and the back link navigate in place; modified clicks
// (new tab, new window) keep the browser's default.
document.addEventListener("click", (event) => {
	const link = event.target instanceof Element ? event.target.closest("a") : null;
	const plain = event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey;
	if (!link || !plain || event.defaultPrevented || link.origin !== location.origin || link.target) return;
	if (link.pathname !== "/" && !conversationPath.test(link.pathname)) return;
	event.preventDefault();
	if (link.pathname === location.pathname) return;
	history.pushState(null, "", link.pathname);
	route({ animate: true });
});

// Back, Forward, and the phone's back gesture. Never push history from here.
addEventListener("popstate", () => route({ animate: true }));

// ── Start ─────────────────────────────────────────────────────────────────────────────

try {
	connect();
	await loadConversations();
	listReady = true;
	for (const [message, isNew] of early.splice(0)) applyLiveMessage(message, isNew);
	route({ animate: false });
	fillPreviews();
} catch (error: unknown) {
	status.textContent = `Couldn’t load Conversations: ${error instanceof Error ? error.message : error}. Reload to try again.`;
}
