// The page's behaviour. `go generate` type-checks this file and compiles it to app.js (tsc
// with erasableSyntaxOnly, so compiling only strips the types); the Proxy embeds app.js.

// ── The Proxy's JSON ──────────────────────────────────────────────────────────────────

interface Conversation {
	id: number;
	name: string;
	service: string; // "iMessage", "SMS", or "RCS"
	isGroup: boolean;
	lastMessageAt: string;
}

interface Attachment {
	name: string;
	kind: "image" | "video" | "audio" | "sticker" | "file";
	isMissing?: boolean; // not on the Host Mac's disk: Messages.app downloads attachments only when asked
}

interface Reaction {
	emoji: string;
	from: string;
	isFromMe?: boolean;
}

type Delivery = "sending" | "unconfirmed" | "failed";

interface Message {
	id: number; // database row ID; negative for a bubble I'm still sending
	guid: string;
	conversationId: number;
	isFromMe: boolean;
	isUnread?: boolean; // received, and unread on all my Apple devices as far as the Host Mac knows
	sender: string;
	senderName?: string;
	isUnknownSender?: boolean; // not in my contacts: no notification, like Messages.app's unknown-senders filter
	text: string;
	createdAt: string;
	isEdited?: boolean;
	reactions?: Reaction[];
	attachments?: Attachment[];
	placeholder?: string; // what the page can't show yet (attachments, polls, …)
	reactsTo?: string; // a live tapback: the GUID of the Message it reacts to
	tapback?: string;
	isTapbackRemoved?: boolean;
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
const notifyButton = find(document, "#notify", HTMLButtonElement);

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
	return row;
}

function showWhen(row: HTMLLIElement, at: string): void {
	const when = find(row, ".when", HTMLTimeElement);
	when.dateTime = at;
	when.textContent = formatWhen(new Date(at));
}

// A Conversation is unread when its newest Message is one I received, the Host Mac's
// database says it's unread, and I haven't opened the Conversation in this browser since.
// Not imsg's unread count: that counts every unread row, and the Host Mac's database keeps
// old rows unread even after I've replied (74 of 100 Conversations, measured 2026-11-16).
// The page can't mark a Message read on the Host Mac (that needs SIP off), so what I've
// opened here is remembered per browser: Conversation ID → newest Message row ID seen.
const readUpTo = fromJSON<Record<string, number>>(localStorage.getItem("readUpTo") ?? "{}");

function markRead(conversationID: number, newest: Message | undefined): void {
	rows.get(conversationID)?.removeAttribute("data-unread");
	notifications.get(conversationID)?.close();
	if (!newest || newest.id <= (readUpTo[conversationID] ?? 0)) return;
	readUpTo[conversationID] = newest.id;
	localStorage.setItem("readUpTo", JSON.stringify(readUpTo));
}

function showUnread(row: HTMLLIElement, newest: Message): void {
	const isUnread = newest.isUnread === true && newest.id > (readUpTo[newest.conversationId] ?? 0);
	row.toggleAttribute("data-unread", isUnread && openID !== newest.conversationId);
}

// ── Notifications ─────────────────────────────────────────────────────────────────────

// Shown by the page itself, so only while it is open somewhere (a tab or an installed
// window); no service worker and no push service. Browsers offer this on HTTPS and
// loopback only, and Chrome on Android not at all (its constructor throws), which is fine:
// the phone gets its notifications from the iPhone.
const canNotify = "Notification" in window && isSecureContext && !/Android/.test(navigator.userAgent);
const notifications = new Map<number, Notification>(); // by Conversation ID: one each, the newest Message

// Like Messages.app: nothing while I'm looking at the page, and nothing from unknown senders.
function notify(conversation: Conversation, message: Message): void {
	if (!canNotify || Notification.permission !== "granted") return;
	if (message.isFromMe || message.isUnknownSender || document.hasFocus()) return;
	const text = previewText(message);
	const notification = new Notification(conversation.name, {
		body: conversation.isGroup && !message.reactsTo ? `${message.senderName}: ${text}` : text,
		tag: String(conversation.id), // a newer Message replaces the Conversation's older notification
	});
	notifications.set(conversation.id, notification);
	notification.addEventListener("click", () => {
		focus();
		const path = `/conversations/${conversation.id}`;
		if (location.pathname === path) return;
		history.pushState(null, "", path);
		route({ shouldAnimate: false });
	});
}

// Browsers only show the permission prompt in answer to a click, so the list offers a
// button until I've answered it.
function offerNotifications(): void {
	if (!canNotify || Notification.permission !== "default") return;
	notifyButton.hidden = false;
	notifyButton.addEventListener("click", async () => {
		await Notification.requestPermission();
		notifyButton.hidden = Notification.permission !== "default";
	});
}

const preview = (row: HTMLLIElement) => find(row, ".preview", HTMLElement);

function previewText(message: Message): string {
	if (message.reactsTo) return `${message.isFromMe ? "You" : message.senderName} reacted ${message.tapback}`;
	return message.text || message.placeholder || "";
}

// Loading skeletons (styled in index.html): empty rows until the list loads, and empty
// bubbles until a Conversation's Messages load. The loaded content replaces them.
function skeletonRows(): HTMLLIElement[] {
	return Array.from({ length: 12 }, () => {
		const row = fromTemplate(rowTemplate, HTMLLIElement);
		row.ariaHidden = "true";
		return row;
	});
}

function skeletonBubbles(): HTMLLIElement[] {
	return ["55%", "30%", "70%", "40%", "60%", "35%"].map((width) => {
		const bubble = document.createElement("p");
		bubble.className = "bubble";
		const li = item("message skeleton", bubble);
		li.style.setProperty("--skeleton-width", width);
		li.ariaHidden = "true";
		return li;
	});
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
	list.removeAttribute("aria-busy");
	if (openID !== null) rows.get(openID)?.querySelector("a")?.setAttribute("aria-current", "page");
}

// The open Conversation's history goes first: imsg's reads contend, and with previews in
// flight a 0.5 s history read took 2.75 s (measured 2026-11-12).
let conversationLoading: Promise<unknown> = Promise.resolve();

// Previews need one request per Conversation, so they fill in after the list shows. One
// fill runs at a time; a fill requested meanwhile runs after it (e.g. after a list reload).
let filling: Promise<void> | null = null;
let shouldFillAgain = false;

function fillPreviews(): void {
	if (filling) {
		shouldFillAgain = true;
		return;
	}
	filling = fillOnce().finally(() => {
		filling = null;
		if (shouldFillAgain) {
			shouldFillAgain = false;
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
			if (!last) {
				preview(row).textContent = "\u00A0"; // nothing to show: not empty, so no loading skeleton
				continue;
			}
			noteSeen(last);
			preview(row).textContent = previewText(last);
			showUnread(row, last);
		}
	};
	// Two workers leave two of imsg's four read lanes free, so opening a Conversation never waits behind previews.
	await Promise.all([worker(), worker()]);
}

// ── The open Conversation ─────────────────────────────────────────────────────────────

let openID: number | null = null;
let openMessages: Message[] = []; // the open Conversation's real Messages
// Bubbles I'm sending (or that came back unconfirmed or failed), per Conversation. Kept apart
// from openMessages, so loading history or switching Conversations never drops one.
const pendingSends = new Map<number, Message[]>();
const pendingFor = (id: number) => pendingSends.get(id) ?? [];

function removePending(conversationID: number, pending: Message): void {
	pendingSends.set(conversationID, pendingFor(conversationID).filter((m) => m !== pending));
}
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
		const isNewDivider = !previous || at.getTime() - new Date(previous.createdAt).getTime() > dividerGap;
		if (isNewDivider) {
			const [day, time] = dividerParts(at);
			const bold = document.createElement("b");
			bold.textContent = day;
			items.push(item("divider", bold, ` ${time}`));
		}
		const isNewSender = isNewDivider || previous?.isFromMe !== message.isFromMe || previous?.sender !== message.sender;
		if (conversation.isGroup && !message.isFromMe && isNewSender) items.push(item("sender", message.senderName ?? ""));
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

const kindLabels: Record<Attachment["kind"], string> = {
	image: "📷 Photo",
	video: "🎬 Video",
	audio: "🎤 Audio",
	sticker: "Sticker",
	file: "📎 File",
};

// An attachment on the Host Mac's disk is shown (images inline, media with native controls,
// files as downloads); one that isn't says so, instead of vanishing.
function renderAttachment(message: Message, attachment: Attachment, index: number): HTMLElement {
	const url = `/api/conversations/${message.conversationId}/messages/${message.id}/attachments/${index}`;
	if (attachment.isMissing) {
		const label = document.createElement("span");
		label.className = "placeholder";
		label.textContent = `${kindLabels[attachment.kind]}, not downloaded on the Mac`;
		return label;
	}
	switch (attachment.kind) {
		case "image":
		case "sticker": {
			const img = document.createElement("img");
			img.src = url;
			img.alt = attachment.name;
			img.loading = "lazy";
			img.addEventListener("load", keepAtEnd);
			return img;
		}
		case "video":
		case "audio": {
			const media = document.createElement(attachment.kind);
			media.src = url;
			media.controls = true;
			media.preload = "metadata";
			return media;
		}
		case "file": {
			const link = document.createElement("a");
			link.href = url;
			link.download = attachment.name;
			link.textContent = `📎 ${attachment.name}`;
			return link;
		}
	}
}

function renderMessage(conversation: Conversation, message: Message): HTMLLIElement {
	const li = item("message");
	li.classList.toggle("from-me", message.isFromMe);
	li.classList.toggle("imessage", conversation.service === "iMessage");
	li.dataset["id"] = String(message.id);

	const attachments = message.attachments ?? [];
	const figures = attachments.map((attachment, index) => {
		const figure = document.createElement("figure");
		figure.className = "attachment";
		figure.append(renderAttachment(message, attachment, index));
		return figure;
	});
	li.append(...figures);

	// The placeholder names attachments when there are any; they're shown above instead.
	const placeholderText = attachments.length > 0 ? undefined : message.placeholder;
	let bubble: HTMLParagraphElement | undefined;
	if (message.text || placeholderText) {
		bubble = document.createElement("p");
		bubble.className = "bubble";
		if (message.text) bubble.append(linkified(message.text));
		if (placeholderText) {
			const placeholder = document.createElement("span");
			placeholder.className = "placeholder";
			placeholder.textContent = placeholderText;
			bubble.append(placeholder);
		}
		li.append(bubble);
	}
	// Tapbacks sit on the bubble, or on the photo when there's no text, as on iOS.
	const tapbackHost = bubble ?? figures.at(-1);
	if (message.reactions?.length && tapbackHost) {
		const tapbacks = document.createElement("span");
		tapbacks.className = "tapbacks";
		tapbacks.textContent = message.reactions.map((r) => r.emoji).join("");
		tapbacks.title = message.reactions.map((r) => `${r.from}: ${r.emoji}`).join("\n");
		tapbackHost.append(tapbacks);
	}
	if (message.delivery) {
		const label = small("delivery", deliveryLabels[message.delivery]);
		label.classList.toggle("warning", message.delivery !== "sending");
		li.append(label);
	}
	if (message.isEdited) li.append(small("edited", "Edited"));
	return li;
}

// Whether the Message list is scrolled to its end; images that load later keep it there,
// unless I've scrolled up to read.
let isAtEnd = true;

function keepAtEnd(): void {
	const messages = main.querySelector("#messages");
	if (isAtEnd && messages) messages.scrollTop = messages.scrollHeight;
}

const isNearEnd = (list: HTMLElement) => list.scrollHeight - list.scrollTop - list.clientHeight < 80;

function showMessages(conversation: Conversation, { shouldScrollToEnd }: { shouldScrollToEnd: boolean }): void {
	const messages = find(main, "#messages", HTMLOListElement);
	const wasAtEnd = isNearEnd(messages);
	messages.replaceChildren(...renderMessages(conversation, [...openMessages, ...pendingFor(conversation.id)]));
	if (shouldScrollToEnd || wasAtEnd) {
		messages.scrollTop = messages.scrollHeight;
		isAtEnd = true;
	}
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
	const messages = find(main, "#messages", HTMLOListElement);
	messages.append(...skeletonBubbles());
	messages.addEventListener("scroll", () => (isAtEnd = isNearEnd(messages)), { passive: true });
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
	markRead(conversation.id, messages.at(-1));
	openMessages = messages;
	showMessages(conversation, { shouldScrollToEnd: true });
}

// ── Sending ───────────────────────────────────────────────────────────────────────────

const shouldSendOnEnter = matchMedia("(pointer: fine)").matches; // desktop, like Messages.app
let pendingCount = 0;

function setUpComposer(conversation: Conversation): void {
	const form = find(main, ".composer", HTMLFormElement);
	const textarea = find(form, "textarea", HTMLTextAreaElement);
	textarea.placeholder = conversation.service === "iMessage" ? "iMessage" : `Text Message • ${conversation.service}`;
	textarea.addEventListener("keydown", (event) => {
		if (shouldSendOnEnter && event.key === "Enter" && !event.shiftKey && !event.isComposing) {
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
			isFromMe: true,
			sender: "",
			text,
			createdAt: new Date().toISOString(),
			delivery: "sending",
		};
		pendingSends.set(conversation.id, [...pendingFor(conversation.id), pending]);
		showMessages(conversation, { shouldScrollToEnd: true });
		const delivery = await send(conversation.id, text, pending);
		if (delivery) pending.delivery = delivery;
		if (openID === conversation.id) showMessages(conversation, { shouldScrollToEnd: false });
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
		// Confirmed: it's a real Message now. The live stream may already have delivered it.
		removePending(conversationID, pending);
		if (openID === conversationID && !openMessages.some((m) => m.id === result.id)) {
			const { delivery: _sending, ...sent } = pending;
			openMessages.push({ ...sent, id: result.id });
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
		// A Message that just arrived is unread until I open its Conversation; one I sent
		// (from any device) means I've read what came before it.
		if (openID === id) markRead(id, message);
		else row.toggleAttribute("data-unread", !message.isFromMe);
		notify(conversation, message);
	}
	if (openID !== id) return;
	if (message.reactsTo) {
		applyTapback(conversation, message);
		return;
	}
	if (message.isFromMe) {
		// My own send arriving live, before the POST answered or after it came back
		// unconfirmed (it was delivered after all): it replaces the pending bubble.
		const pending = pendingFor(id).find((m) => (m.delivery === "sending" || m.delivery === "unconfirmed") && m.text.trim() === message.text);
		if (pending) removePending(id, pending);
	}
	const index = openMessages.findIndex((m) => m.id === message.id);
	if (index >= 0) openMessages[index] = message; // an edit, or a replay after reconnecting
	else openMessages.push(message);
	showMessages(conversation, { shouldScrollToEnd: message.isFromMe });
}

// A tapback is its own row pointing at the Message it reacts to (by GUID).
function applyTapback(conversation: Conversation, tapback: Message): void {
	const target = openMessages.find((m) => m.guid === tapback.reactsTo);
	if (!target || !tapback.tapback) return; // reacting to a Message older than the 100 shown
	const emoji = tapback.tapback;
	const from = tapback.isFromMe ? "You" : (tapback.senderName ?? "");
	target.reactions = (target.reactions ?? []).filter((r) => !(r.from === from && r.emoji === emoji));
	if (!tapback.isTapbackRemoved) target.reactions.push({ emoji, from, isFromMe: tapback.isFromMe });
	showMessages(conversation, { shouldScrollToEnd: false });
}

const quietLimit = 60_000; // the Proxy sends a heartbeat every 25 s
let source: EventSource | undefined;
let watchdog: number | undefined;

// Subscribe before loading, so a Message arriving meanwhile isn't lost; hold it until the
// list exists.
let isListReady = false;
const early: [Message, boolean][] = [];

function setOnline(isOnline: boolean): void {
	connection.hidden = isOnline;
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
		if (isListReady) applyLiveMessage(message, isNew);
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
function route({ shouldAnimate }: { shouldAnimate: boolean }): void {
	const id = conversationIDFromPath();
	const previousID = openID;
	if (!shouldAnimate || reduceMotion.matches) {
		showConversation(id);
		loadOpenConversation(id);
		moveFocus(id, previousID);
		return;
	}
	document.documentElement.dataset["direction"] = id === null ? "back" : "forward";
	const transition = document.startViewTransition(() => showConversation(id));
	// Messages load as soon as the frame is in place; only focus waits for the animation.
	transition.updateCallbackDone.then(() => loadOpenConversation(id));
	transition.finished.finally(() => moveFocus(id, previousID));
}

// A navigation replaced by a newer one (e.g. Back during the animation) does nothing.
const isCurrent = (id: number | null) => id === openID;

function loadOpenConversation(id: number | null): void {
	if (!isCurrent(id)) return;
	loadConversation(id).catch((error: unknown) => {
		main.replaceChildren(status);
		status.textContent = `Couldn’t load this Conversation: ${error instanceof Error ? error.message : error}. Reload to try again.`;
	});
}

// Focus follows the navigation (a View Transition doesn't move it). Returning to the list
// focuses the row I came from, scrolling it into view, so I keep my place.
function moveFocus(id: number | null, previousID: number | null): void {
	if (!isCurrent(id)) return;
	if (id !== null) {
		main.querySelector("h2")?.focus({ preventScroll: true });
	} else if (previousID !== null) {
		rows.get(previousID)?.querySelector("a")?.focus();
	}
}

// Plain clicks on Conversation links and the back link navigate in place; modified clicks
// (new tab, new window) keep the browser's default. Opening a Conversation from the list
// marks the history entry, so the back link can go back instead of stacking a new "/".
function onClick(event: MouseEvent): void {
	const link = event.target instanceof Element ? event.target.closest("a") : null;
	const isPlainClick = event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey;
	if (!link || !isPlainClick || event.defaultPrevented || link.origin !== location.origin || link.target) return;
	if (link.pathname !== "/" && !conversationPath.test(link.pathname)) return;
	event.preventDefault();
	if (link.pathname === location.pathname) return;
	const state: unknown = history.state;
	if (link.pathname === "/" && typeof state === "object" && state !== null && "fromList" in state) {
		history.back(); // popstate routes
		return;
	}
	history.pushState(location.pathname === "/" ? { fromList: true } : null, "", link.pathname);
	route({ shouldAnimate: true });
}

// Back, Forward, and the phone's back gesture. Never push history from here. When the
// browser already animated the swipe, don't animate it a second time.
function onPopState(event: PopStateEvent): void {
	route({ shouldAnimate: !event.hasUAVisualTransition });
}

// ── Start ─────────────────────────────────────────────────────────────────────────────

try {
	list.replaceChildren(...skeletonRows());
	connect();
	await loadConversations();
	isListReady = true;
	for (const [message, isNew] of early.splice(0)) applyLiveMessage(message, isNew);
	document.addEventListener("click", onClick);
	addEventListener("popstate", onPopState);
	route({ shouldAnimate: false });
	fillPreviews();
	offerNotifications();
} catch (error: unknown) {
	list.replaceChildren();
	status.textContent = `Couldn’t load Conversations: ${error instanceof Error ? error.message : error}. Reload to try again.`;
}
