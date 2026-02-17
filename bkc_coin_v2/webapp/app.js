/* global React, ReactDOM */

const { useEffect, useMemo, useRef, useState } = React;

const tg = window.Telegram?.WebApp;
if (tg) {
  tg.ready();
  tg.expand();
  try {
    tg.setHeaderColor("#0a3a4e");
    tg.setBackgroundColor("#12253f");
  } catch {}
}

const format = (value) => Number(value || 0).toLocaleString("ru-RU");
const qp = new URLSearchParams(window.location.search);
const apiParam = qp.get("api");
const nodesParam = qp.get("nodes");
const nodesTapParam = qp.get("nodes_tap");
const nodesMarketParam = qp.get("nodes_market");
const nodesBankParam = qp.get("nodes_bank");
const nodesAdminParam = qp.get("nodes_admin");
const nodesFullParam = qp.get("nodes_full");
const fallbackApi =
  window.location.hostname === "127.0.0.1" || window.location.hostname === "localhost"
    ? "http://127.0.0.1:8080"
    : window.location.origin;

function clampInt(v, min, max, def = min) {
  const x = Number(v);
  if (!Number.isFinite(x)) return def;
  return Math.max(min, Math.min(max, Math.floor(x)));
}

function normalizeBase(url) {
  return String(url || "").trim().replace(/\/$/, "");
}

function parseNodes(raw) {
  const s = String(raw || "").trim();
  if (!s) return [];
  return s
    .split(",")
    .map((x) => normalizeBase(x))
    .filter(Boolean);
}

const fromWindow = Array.isArray(window.BKC_API_NODES) ? window.BKC_API_NODES : [];
const configuredPool = Array.from(
  new Set([
    ...parseNodes(nodesParam),
    ...fromWindow.map((x) => normalizeBase(x)).filter(Boolean),
  ])
);
const nodesTapWindow = Array.isArray(window.BKC_API_NODES_TAP) ? window.BKC_API_NODES_TAP : [];
const nodesMarketWindow = Array.isArray(window.BKC_API_NODES_MARKET) ? window.BKC_API_NODES_MARKET : [];
const nodesBankWindow = Array.isArray(window.BKC_API_NODES_BANK) ? window.BKC_API_NODES_BANK : [];
const nodesAdminWindow = Array.isArray(window.BKC_API_NODES_ADMIN) ? window.BKC_API_NODES_ADMIN : [];
const nodesFullWindow = Array.isArray(window.BKC_API_NODES_FULL) ? window.BKC_API_NODES_FULL : [];

function uniqNodes(...parts) {
  return Array.from(
    new Set(
      parts
        .flat()
        .map((x) => normalizeBase(x))
        .filter(Boolean)
    )
  );
}

const defaultPool = apiParam
  ? [normalizeBase(apiParam)]
  : configuredPool.length > 0
  ? configuredPool
  : [normalizeBase(fallbackApi)];

const POOLS = {
  full: uniqNodes(parseNodes(nodesFullParam), nodesFullWindow, defaultPool),
  tap: uniqNodes(parseNodes(nodesTapParam), nodesTapWindow, defaultPool),
  market: uniqNodes(parseNodes(nodesMarketParam), nodesMarketWindow, defaultPool),
  bank: uniqNodes(parseNodes(nodesBankParam), nodesBankWindow, defaultPool),
  admin: uniqNodes(parseNodes(nodesAdminParam), nodesAdminWindow, defaultPool),
};

function endpointProfile(path) {
  const p = String(path || "").toLowerCase().trim();
  if (!p) return "full";
  if (
    p === "tap" ||
    p === "state" ||
    p === "buy" ||
    p === "blockchain" ||
    p === "health"
  ) {
    return "tap";
  }
  if (p.startsWith("market/") || p.startsWith("nfts/") || p.startsWith("assets/listings/")) {
    return "market";
  }
  if (
    p === "transfer" ||
    p.startsWith("bank/") ||
    p.startsWith("p2p/") ||
    p.startsWith("deposit/") ||
    p.startsWith("cryptopay/")
  ) {
    return "bank";
  }
  if (p.startsWith("admin/")) {
    return "admin";
  }
  return "full";
}

function pickApiBase(profile) {
  const mode = profile || "full";
  const pool = POOLS[mode] && POOLS[mode].length ? POOLS[mode] : defaultPool;
  const key = `bkc_api_base_${mode}`;
  const stored = normalizeBase(sessionStorage.getItem(key));
  if (stored && pool.includes(stored)) {
    return stored;
  }
  const uid = Number(tg?.initDataUnsafe?.user?.id || 0);
  let idx = 0;
  if (Number.isFinite(uid) && uid > 0) {
    idx = Math.abs(uid) % pool.length;
  } else {
    idx = Math.floor(Math.random() * pool.length);
  }
  const chosen = pool[idx];
  sessionStorage.setItem(key, chosen);
  return chosen;
}

function getApiBaseForPath(path) {
  const profile = endpointProfile(path);
  return pickApiBase(profile);
}

function rotateNode(path, currentBase) {
  const profile = endpointProfile(path);
  const mode = profile || "full";
  const pool = POOLS[mode] && POOLS[mode].length ? POOLS[mode] : defaultPool;
  if (pool.length <= 1) return currentBase;
  const idx = pool.indexOf(currentBase);
  const next = pool[(idx + 1 + pool.length) % pool.length];
  sessionStorage.setItem(`bkc_api_base_${mode}`, next);
  return next;
}

let LAST_API_BASE = "";
const DEFAULT_COIN_IMG = `${getApiBaseForPath("state")}/assets/coin.svg`;

const MAX_MULTITOUCH = 13;
const TAP_BATCH_WINDOW_MS = 90;
const TAP_SYNC_INTERVAL_MS = clampInt(qp.get("tap_sync_ms"), 200, 10_000, 650);
const MAX_TAP_BATCH = 500;

async function requestJSON(method, path, payload = undefined) {
  const profile = endpointProfile(path);
  const pool = POOLS[profile] && POOLS[profile].length ? POOLS[profile] : defaultPool;
  let attempt = 0;
  const maxAttempts = Math.min(3, pool.length);
  let lastErr = null;
  let base = getApiBaseForPath(path);

  while (attempt < maxAttempts) {
    try {
      LAST_API_BASE = base;
      const response = await fetch(`${base}/api/v1/${path}`, {
        method,
        headers: { "Content-Type": "application/json" },
        body: payload === undefined ? undefined : JSON.stringify(payload),
      });
      const data = await response.json();
      if (!response.ok || !data.ok) {
        throw new Error(data.error || "Ошибка API");
      }
      return data.data;
    } catch (e) {
      lastErr = e;
      attempt += 1;
      if (attempt < maxAttempts) {
        base = rotateNode(path, base);
      }
    }
  }

  throw lastErr || new Error("Ошибка сети");
}

async function post(path, payload = {}) {
  return requestJSON("POST", path, {
    init_data: tg?.initData || "",
    ...payload,
  });
}

async function get(path) {
  return requestJSON("GET", path);
}

function App() {
  const initialTab = (new URLSearchParams(window.location.search).get("tab") || "tap").toLowerCase();
  const [tab, setTab] = useState(["tap", "wallet", "market", "deposit", "chain"].includes(initialTab) ? initialTab : "tap");
  const [loading, setLoading] = useState(true);
  const [state, setState] = useState(null);
  const [chain, setChain] = useState(null);
  const [message, setMessage] = useState(null);
  const [floating, setFloating] = useState([]);

  // Language state
  const [language, setLanguage] = useState(() => {
    const saved = localStorage.getItem('bkc_language');
    return saved || 'ru';
  });

  const texts = {
    ru: {
      loading: 'Загрузка...',
      connecting: 'Подключаюсь к backend API',
      telegramOnly: 'Mini App работает только в Telegram',
      openInBot: 'Откройте MINI APP в боте',
      tapEngine: 'Тап движок',
      energy: 'Energy',
      taps: 'Taps',
      bkcPerDollar: 'BKC / $1',
      refresh: 'Обновить',
      sync: 'Синхронизировать',
      wallet: 'Кошелек',
      market: 'Маркет',
      deposit: 'Пополнить',
      chain: 'Цепочка',
      level: 'Level',
      bkc: 'BKC',
      available: 'Доступно',
      frozen: 'Заморожено',
      nftBought: 'NFT куплен',
      transferComplete: 'Перевод выполнен',
      energyActivated: 'Energy 1h активирована. Списано {cost} BKC.',
      limitIncreased: 'Лимит на сегодня увеличен на +{size}. Списано {cost} BKC.',
      fundsFrozen: 'Заморожено {amount} BKC',
      fundsUnfrozen: 'Разморожено {amount} BKC',
      dailyLimitExceeded: 'Дневной лимит тапов исчерпан. Можно купить доп. лимит в Market.',
      reserveEmpty: 'Резерв пуст. Сейчас нельзя майнить через тап.',
      close: 'Закрыть',
      daily: 'Сегодня: {current} / {max} • Остаток: {remaining}',
      multiTouch: 'Поддержка до {max} пальцев. Тапы тратят энергию.',
      levelText: 'Level',
      availableFrozen: 'Доступно: {available} BKC • Заморожено: {frozen} BKC',
      creditTaken: 'Кредит оформлен: +{amount} BKC',
      loanRepaid: 'Кредит погашен',
      loanRequested: 'Заявка отправлена #{id}',
      loanIssued: 'Долг выдан',
      loanRejected: 'Заявка отклонена',
      loanRepaidP2P: 'Долг погашен',
      loanRecalled: 'Долг возвращен (если у заемщика хватило средств)',
      listingCreated: 'Объявление создано. Комиссия: {fee} BKC сожжено.',
      listingDeleted: 'Объявление удалено',
      listingCancelled: 'Объявление снято',
      purchaseCompleted: 'Покупка выполнена',
      requestCreated: 'Заявка #{id} создана. Жди подтверждение админа.',
      depositApproved: 'Пополнение подтверждено',
      depositRejected: 'Пополнение отклонено',
      walletsUpdated: 'Кошельки обновлены',
      broadcastSent: 'Рассылка запущена. Статус придет в Telegram.',
      invoiceCreated: 'Инвойс создан: ${usd}$ => +{coins} BKC',
      statusUpdated: 'Статус: {status}. Начислено: {credited} BKC',
      walletSaved: 'Кошельки сохранены',
      depositCreated: 'Заявка #{deposit_id} создана. Ожидайте подтверждения.',
      // Navigation labels
      tapTab: 'Тап',
      walletTab: 'Кошелек',
      marketTab: 'Маркет',
      depositTab: 'Пополнить',
      chainTab: 'Цепочка',
      // Panel titles
      blockchain: 'Блокчейн',
      // Labels
      users: 'Пользователи',
      transactions: 'Транзакции',
      tapped: 'Накликано',
      reserve: 'Резерв',
      energy1h: 'Energy 1ч',
      nftShop: 'Магазин NFT',
      admin: 'Админ',
      invoice: 'Инвойс',
      usdt: 'USDT',
      trx: 'TRX',
      sol: 'SOL',
      depositToWallet: 'Пополнение по кошельку',
      // Additional texts
      noNft: 'Пока нет NFT.',
      bank: 'Банк',
      creditInfo: 'Кредит можно погасить в любое время. Если просрочишь, система автоматически спишет долг и баланс может уйти в минус.',
      amount: 'Сумма',
      takeLoan7d: 'Взять 7д',
      takeLoan30d: 'Взять 30д',
      myLoans: 'Мои кредиты',
      updateBalance: 'Обновить баланс',
      example: 'Пример',
      loanTotal: 'всего к возврату',
      editWallets: 'Редактировать кошельки',
      key: 'Ключ',
      address: 'Адрес',
      add: 'Добавить',
      save: 'Сохранить',
      freezeFunds: 'Заморозить',
      unfreezeFunds: 'Разморозить',
      transfer: 'Перевод',
      recipientAddress: 'Адрес получателя',
      send: 'Отправить',
      close: 'Закрыть',
      sumInUsd: 'Сумма в USD',
      txHash: 'TX hash',
      createRequest: 'Создать заявку',
      requests: 'заявки',
      update: 'Обновить',
      confirm: 'Подтвердить',
      reject: 'Отклонить',
      noPendingRequests: 'Нет pending заявок.',
      deposit: 'Пополнение',
      rate: 'Курс',
      cryptobot: 'CryptoBot (CryptoPay)',
      createInvoice: 'Создать инвойс',
      check: 'Проверить',
      adminConfirmation: 'админ подтверждает)',
      walletsNotConfigured: 'Кошельки для пополнения не настроены.',
      clickToCopy: 'Нажми чтобы скопировать',
      addressCopied: 'адрес скопирован',
      updateNetwork: 'Обновить сеть',
      miniAppTelegramOnly: 'Mini App доступен только в Telegram',
      openInBot: 'Открой через кнопку MINI APP в боте.',
    },
    en: {
      loading: 'Loading...',
      connecting: 'Connecting to backend API',
      telegramOnly: 'Mini App works only in Telegram',
      openInBot: 'Open MINI APP in bot',
      tapEngine: 'Tap Engine',
      energy: 'Energy',
      taps: 'Taps',
      bkcPerDollar: 'BKC / $1',
      refresh: 'Refresh',
      sync: 'Sync',
      wallet: 'Wallet',
      market: 'Market',
      deposit: 'Deposit',
      chain: 'Chain',
      level: 'Level',
      bkc: 'BKC',
      available: 'Available',
      frozen: 'Frozen',
      nftBought: 'NFT purchased',
      transferComplete: 'Transfer completed',
      energyActivated: `Energy 1h activated. Charged ${format(0)} BKC.`,
      limitIncreased: `Daily limit increased by +${format(0)}. Charged ${format(0)} BKC.`,
      fundsFrozen: `Frozen ${format(0)} BKC`,
      fundsUnfrozen: `Unfrozen ${format(0)} BKC`,
      dailyLimitExceeded: 'Daily tap limit exceeded. You can buy extra limit in Market.',
      reserveEmpty: 'Reserve is empty. Cannot mine through taps now.',
      creditTaken: 'Credit issued: +{amount} BKC',
      loanRepaid: 'Loan repaid',
      loanRequested: 'Application sent #{id}',
      loanIssued: 'Debt issued',
      loanRejected: 'Application rejected',
      loanRepaidP2P: 'Debt repaid',
      loanRecalled: 'Debt returned (if borrower had enough funds)',
      listingCreated: 'Listing created. Fee: {fee} BKC burned.',
      listingDeleted: 'Listing deleted',
      listingCancelled: 'Listing cancelled',
      purchaseCompleted: 'Purchase completed',
      requestCreated: 'Application #{id} created. Waiting for admin confirmation.',
      depositApproved: 'Deposit approved',
      depositRejected: 'Deposit rejected',
      walletsUpdated: 'Wallets updated',
      broadcastSent: 'Broadcast launched. Status will come to Telegram.',
      invoiceCreated: 'Invoice created: ${usd}$ => +{coins} BKC',
      statusUpdated: 'Status: {status}. Credited: {credited} BKC',
      walletSaved: 'Wallets saved',
      depositCreated: 'Application #{deposit_id} created. Waiting for confirmation.',
      // Navigation labels
      tapTab: 'Tap',
      walletTab: 'Wallet',
      marketTab: 'Market',
      depositTab: 'Deposit',
      chainTab: 'Chain',
      // Panel titles
      blockchain: 'Blockchain',
      // Labels
      users: 'Users',
      transactions: 'Txs',
      tapped: 'Tapped',
      reserve: 'Reserve',
      energy1h: 'Energy 1h',
      nftShop: 'NFT Shop',
      admin: 'Admin',
      invoice: 'Invoice',
      usdt: 'USDT',
      trx: 'TRX',
      sol: 'SOL',
      depositToWallet: 'Deposit to wallet',
      // Additional texts
      noNft: 'No NFT yet.',
      bank: 'Bank',
      creditInfo: 'Credit can be repaid at any time. If you default, the system will automatically deduct the debt and your balance may go negative.',
      amount: 'Amount',
      takeLoan7d: 'Take 7d',
      takeLoan30d: 'Take 30d',
      myLoans: 'My loans',
      updateBalance: 'Update balance',
      example: 'Example',
      loanTotal: 'total to return',
      editWallets: 'Edit wallets',
      key: 'Key',
      address: 'Address',
      add: 'Add',
      save: 'Save',
      freezeFunds: 'Freeze',
      unfreezeFunds: 'Unfreeze',
      transfer: 'Transfer',
      recipientAddress: 'Recipient address',
      send: 'Send',
      sumInUsd: 'Sum in USD',
      txHash: 'TX hash',
      createRequest: 'Create request',
      requests: 'requests',
      update: 'Update',
      confirm: 'Confirm',
      reject: 'Reject',
      noPendingRequests: 'No pending requests.',
      deposit: 'Deposit',
      rate: 'Rate',
      cryptobot: 'CryptoBot (CryptoPay)',
      createInvoice: 'Create invoice',
      check: 'Check',
      adminConfirmation: 'admin confirms)',
      walletsNotConfigured: 'Deposit wallets are not configured.',
      clickToCopy: 'Click to copy',
      addressCopied: 'address copied',
      updateNetwork: 'Update network',
      miniAppTelegramOnly: 'Mini App available only in Telegram',
      openInBot: 'Open via MINI APP button in bot.',
    }
  };

  const t = (key) => texts[language][key] || key;

  const toggleLanguage = () => {
    const newLang = language === 'ru' ? 'en' : 'ru';
    setLanguage(newLang);
    localStorage.setItem('bkc_language', newLang);
  };

  // Continue with the rest of the component logic...
  
  // Additional states
  const [tapCount, setTapCount] = useState(0);
  const [energy, setEnergy] = useState(0);
  const [maxEnergy, setMaxEnergy] = useState(0);
  const [dailyTaps, setDailyTaps] = useState(0);
  const [maxDailyTaps, setMaxDailyTaps] = useState(0);
  const [tapValue, setTapValue] = useState(0);
  const [rate, setRate] = useState(0);
  const [coinImg, setCoinImg] = useState(DEFAULT_COIN_IMG);
  const [freezeAmount, setFreezeAmount] = useState("");
  const [loanAmount, setLoanAmount] = useState("");
  const [loan7BP, setLoan7BP] = useState(0);
  const [loan30BP, setLoan30BP] = useState(0);
  const [loanMax, setLoanMax] = useState(0);
  const [bankLoans, setBankLoans] = useState([]);
  const [depositCurrency, setDepositCurrency] = useState("USDT");
  const [depositUsd, setDepositUsd] = useState("");
  const [depositTx, setDepositTx] = useState("");
  const [invoice, setInvoice] = useState(null);
  const [walletEditorOpen, setWalletEditorOpen] = useState(false);
  const [walletDraft, setWalletDraft] = useState([]);
  const [adminDeposits, setAdminDeposits] = useState([]);
  const [marketListings, setMarketListings] = useState([]);
  const [marketNfts, setMarketNfts] = useState([]);
  const [myListings, setMyListings] = useState([]);
  const [myNfts, setMyNfts] = useState([]);
  const [listingAmount, setListingAmount] = useState("");
  const [listingPrice, setListingPrice] = useState("");
  const [selectedNft, setSelectedNft] = useState(null);
  const [transferTo, setTransferTo] = useState("");
  const [transferAmount, setTransferAmount] = useState("");
  const [energy1hAmount, setEnergy1hAmount] = useState("");
  const [limitAmount, setLimitAmount] = useState("");
  const [broadcastText, setBroadcastText] = useState("");
  const [tapEngine, setTapEngine] = useState(false);
  const [tapBatch, setTapBatch] = useState([]);
  const [tapBatchTimer, setTapBatchTimer] = useState(null);
  const [lastTapSync, setLastTapSync] = useState(0);
  const tapZoneRef = useRef(null);
  const coinRef = useRef(null);

  // Utility functions
  const calcLoanTotal = (amount, bp) => {
    return Math.floor(amount * (1 + bp / 10000));
  };

  const updateWalletRow = (idx, field, value) => {
    const updated = [...walletDraft];
    if (!updated[idx]) {
      updated[idx] = { k: "", v: "" };
    }
    updated[idx][field] = value;
    setWalletDraft(updated);
  };

  const removeWalletRow = (idx) => {
    setWalletDraft(walletDraft.filter((_, i) => i !== idx));
  };

  const addWalletRow = () => {
    setWalletDraft([...walletDraft, { k: "", v: "" }]);
  };

  // API functions
  async function loadState() {
    try {
      const data = await get("state");
      setState(data);
      setEnergy(data.energy || 0);
      setMaxEnergy(data.max_energy || 0);
      setDailyTaps(data.daily_taps || 0);
      setMaxDailyTaps(data.max_daily_taps || 0);
      setTapValue(data.tap_value || 0);
      setRate(data.rate || 0);
      setCoinImg(data.coin_img || DEFAULT_COIN_IMG);
      setLoan7BP(data.loan_7d_bp || 0);
      setLoan30BP(data.loan_30d_bp || 0);
      setLoanMax(data.loan_max || 0);
      setMessage(null);
    } catch (e) {
      setMessage({ type: "bad", text: e.message });
    } finally {
      setLoading(false);
    }
  }

  async function loadChain() {
    try {
      const data = await get("blockchain");
      setChain(data);
      setMessage(null);
    } catch (e) {
      setMessage({ type: "bad", text: e.message });
    }
  }

  async function tap() {
    if (!tapEngine || energy <= 0) return;
    
    const now = Date.now();
    setTapBatch(prev => [...prev, now]);
    
    if (tapBatch.length === 1) {
      const timer = setTimeout(() => {
        syncTaps();
      }, TAP_BATCH_WINDOW_MS);
      setTapBatchTimer(timer);
    }
    
    if (tapBatch.length >= MAX_TAP_BATCH) {
      syncTaps();
    }
    
    setEnergy(prev => Math.max(0, prev - tapValue));
    setTapCount(prev => prev + 1);
    
    // Add floating animation
    const rect = coinRef.current?.getBoundingClientRect();
    if (rect) {
      const float = {
        id: Math.random(),
        x: rect.left + rect.width / 2,
        y: rect.top + rect.height / 2,
        value: tapValue
      };
      setFloating(prev => [...prev, float]);
      setTimeout(() => {
        setFloating(prev => prev.filter(f => f.id !== float.id));
      }, 1000);
    }
  }

  async function syncTaps() {
    if (tapBatch.length === 0) return;
    
    try {
      const taps = tapBatch.length;
      await post("tap", { taps });
      setLastTapSync(Date.now());
      setTapBatch([]);
      if (tapBatchTimer) {
        clearTimeout(tapBatchTimer);
        setTapBatchTimer(null);
      }
      await loadState();
    } catch (e) {
      setMessage({ type: "bad", text: e.message });
    }
  }

  async function freezeFunds() {
    try {
      const amount = clampInt(freezeAmount, 1, 500_000_000, 1);
      const data = await post("bank/freeze", { amount });
      setState(data);
      setFreezeAmount("");
      setMessage({ type: "good", text: t('fundsFrozen').replace('{amount}', format(amount)) });
    } catch (e) {
      setMessage({ type: "bad", text: e.message });
    }
  }

  async function unfreezeFunds() {
    try {
      const amount = clampInt(freezeAmount, 1, 500_000_000, 1);
      const data = await post("bank/unfreeze", { amount });
      setState(data);
      setFreezeAmount("");
      setMessage({ type: "good", text: t('fundsUnfrozen').replace('{amount}', format(amount)) });
    } catch (e) {
      setMessage({ type: "bad", text: e.message });
    }
  }

  async function takeLoan(period) {
    try {
      const amount = clampInt(loanAmount, 1, loanMax || 0, 0);
      const data = await post("bank/loan", { amount, period });
      setState(data);
      setLoanAmount("");
      setMessage({ type: "good", text: t('creditTaken').replace('{amount}', format(amount)) });
    } catch (e) {
      setMessage({ type: "bad", text: e.message });
    }
  }

  async function loadBankLoans() {
    try {
      const data = await get("bank/loans");
      setBankLoans(data.loans || []);
      setMessage(null);
    } catch (e) {
      setMessage({ type: "bad", text: e.message });
    }
  }

  async function createDeposit() {
    try {
      const amount = clampInt(depositUsd, 1, 10000, 0);
      const data = await post("deposit/create", { 
        currency: depositCurrency, 
        amount_usd: amount, 
        tx_hash: depositTx 
      });
      setDepositUsd("");
      setDepositTx("");
      setMessage({ type: "good", text: t('depositCreated').replace('{deposit_id}', data.deposit_id) });
    } catch (e) {
      setMessage({ type: "bad", text: e.message });
    }
  }

  async function createCryptoPayInvoice() {
    try {
      const amount = clampInt(depositUsd, 1, 10000, 0);
      const data = await post("cryptopay/invoice", { amount_usd: amount });
      setInvoice(data);
      setMessage({ type: "good", text: t('invoiceCreated').replace('${usd}', amount).replace('{coins}', format(data.coins)) });
    } catch (e) {
      setMessage({ type: "bad", text: e.message });
    }
  }

  async function checkCryptoPayInvoice() {
    try {
      const data = await post("cryptopay/check", { invoice_id: invoice.invoice_id });
      setInvoice(data);
      setMessage({ type: "good", text: t('statusUpdated').replace('{status}', data.status).replace('{credited}', format(data.credited || 0)) });
    } catch (e) {
      setMessage({ type: "bad", text: e.message });
    }
  }

  async function saveWallets() {
    try {
      const wallets = walletDraft.filter(row => row.k && row.v);
      const data = await post("admin/wallets", { wallets });
      setState(data);
      setWalletEditorOpen(false);
      setMessage({ type: "good", text: t('walletSaved') });
    } catch (e) {
      setMessage({ type: "bad", text: e.message });
    }
  }

  async function adminLoadDeposits() {
    try {
      const data = await get("admin/deposits");
      setAdminDeposits(data.deposits || []);
      setMessage(null);
    } catch (e) {
      setMessage({ type: "bad", text: e.message });
    }
  }

  async function adminProcessDeposit(depositId, approve) {
    try {
      await post("admin/deposit", { deposit_id: depositId, approve });
      setAdminDeposits(prev => prev.filter(d => d.deposit_id !== depositId));
      setMessage({ type: "good", text: approve ? t('depositApproved') : t('depositRejected') });
    } catch (e) {
      setMessage({ type: "bad", text: e.message });
    }
  }

  // Effects
  useEffect(() => {
    loadState();
    const interval = setInterval(() => {
      if (Date.now() - lastTapSync > TAP_SYNC_INTERVAL_MS) {
        syncTaps();
      }
    }, TAP_SYNC_INTERVAL_MS);
    
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    if (tab === "chain") {
      loadChain();
    }
  }, [tab]);

  useEffect(() => {
    if (state?.deposit_wallets) {
      setWalletDraft(Object.entries(state.deposit_wallets).map(([k, v]) => ({ k, v })));
    }
  }, [state]);

  // Render functions
  if (!tg?.initData) {
    return (
      <main className="app">
        <section className="card panel">
          <div className="headline">{t('miniAppTelegramOnly')}</div>
          <div className="muted">{t('openInBot')}</div>
        </section>
      </main>
    );
  }

  return (
    <main className="app">
      {/* Header */}
      <header className="header card">
        <div className="user-info">
          <img 
            src={tg?.initDataUnsafe?.user?.photo_url || ""} 
            alt="Avatar" 
            className="avatar"
            onError={(e) => { e.target.style.display = 'none'; }}
          />
          <div className="user-details">
            <div className="name">{tg?.initDataUnsafe?.user?.first_name || "User"}</div>
            <div className="level">{t('levelText')} {state?.level || 1}</div>
          </div>
        </div>
        <div className="balance-section">
          <div className="balance-amount">{format(state?.balance || 0)}</div>
          <div className="balance-label">{t('bkc')}</div>
        </div>
      </header>

      {/* Stats */}
      <div className="stats-grid card">
        <div className="stat-card">
          <div className="stat-value">{format(state?.total_users || 0)}</div>
          <div className="stat-label">{t('users')}</div>
        </div>
        <div className="stat-card">
          <div className="stat-value">{format(state?.total_taps || 0)}</div>
          <div className="stat-label">{t('tapped')}</div>
        </div>
        <div className="stat-card">
          <div className="stat-value">{format(state?.reserve || 0)}</div>
          <div className="stat-label">{t('reserve')}</div>
        </div>
      </div>

      {/* Main Content */}
      {tab === "tap" && (
        <section className="card panel">
          <div className="panel-header">
            <div>
              <div className="panel-title">{t('tapEngine')}</div>
              <div className="panel-subtitle">{t('multiTouch').replace('{max}', MAX_MULTITOUCH)}</div>
            </div>
            <button className="btn btn-secondary" onClick={loadState}>
              {t('refresh')}
            </button>
          </div>

          {/* Energy Bar */}
          <div className="energy-bar">
            <div className="energy-header">
              <span className="energy-label">{t('energy')}</span>
              <span className="energy-value">{format(energy)} / {format(maxEnergy)}</span>
            </div>
            <div className="progress-track">
              <div 
                className="progress-fill" 
                style={{ width: `${maxEnergy > 0 ? (energy / maxEnergy) * 100 : 0}%` }}
              />
            </div>
          </div>

          {/* Tap Zone */}
          <div className="tap-zone" ref={tapZoneRef}>
            <div className="coin-container" onClick={tap} ref={coinRef}>
              <div className="coin-glow" />
              <div className="coin">
                <div className="coin-text">BKC</div>
              </div>
            </div>
            {floating.map(float => (
              <div
                key={float.id}
                className="tap-float"
                style={{
                  position: 'fixed',
                  left: float.x,
                  top: float.y,
                  transform: 'translate(-50%, -50%)'
                }}
              >
                +{format(float.value)}
              </div>
            ))}
          </div>

          <div className="muted text-center mt-16">
            {t('daily').replace('{current}', format(dailyTaps)).replace('{max}', format(maxDailyTaps)).replace('{remaining}', format(Math.max(0, maxDailyTaps - dailyTaps)))}
          </div>
        </section>
      )}

      {tab === "wallet" && (
        <section className="card panel">
          <div className="panel-header">
            <div className="panel-title">{t('wallet')}</div>
            <button className="btn btn-secondary" onClick={loadState}>
              {t('refresh')}
            </button>
          </div>

          <div className="headline">{t('availableFrozen').replace('{available}', format(state?.balance || 0)).replace('{frozen}', format(state?.frozen || 0))}</div>
          
          <div className="form-group">
            <label className="form-label">{t('freezeFunds')}</label>
            <div className="row">
              <input 
                className="input" 
                type="number" 
                min="1" 
                placeholder={t('amount')}
                value={freezeAmount} 
                onChange={(e) => setFreezeAmount(e.target.value)} 
              />
              <div className="grid-2">
                <button className="btn btn-primary" onClick={freezeFunds}>{t('freeze')}</button>
                <button className="btn btn-secondary" onClick={unfreezeFunds}>{t('unfreeze')}</button>
              </div>
            </div>
          </div>

          <div className="headline mt-16">{t('transfer')}</div>
          <div className="form-group">
            <input 
              className="input" 
              placeholder={t('recipientAddress')}
              value={transferTo} 
              onChange={(e) => setTransferTo(e.target.value)} 
            />
            <input 
              className="input" 
              type="number" 
              min="1" 
              placeholder={t('amount')}
              value={transferAmount} 
              onChange={(e) => setTransferAmount(e.target.value)} 
            />
            <button className="btn btn-primary">{t('send')}</button>
          </div>
        </section>
      )}

      {tab === "market" && (
        <section className="card panel">
          <div className="panel-header">
            <div className="panel-title">{t('market')}</div>
            <button className="btn btn-secondary" onClick={loadState}>
              {t('refresh')}
            </button>
          </div>

          <div className="headline">{t('nftShop')}</div>
          <div className="muted">{t('noNft')}</div>

          <div className="headline mt-16">{t('bank')}</div>
          <div className="muted">
            {t('creditInfo')}
          </div>
          <div className="row mt-8">
            <input 
              className="input" 
              type="number" 
              min="1" 
              placeholder={`${t('amount')} (до ${format(loanMax || 0)} BKC)`}
              value={loanAmount} 
              onChange={(e) => setLoanAmount(e.target.value)} 
            />
            <div className="grid-2">
              <button className="btn btn-warning" onClick={() => takeLoan("7d")}>
                {t('takeLoan7d')} ({Math.round((loan7BP || 0) / 100)}%)
              </button>
              <button className="btn btn-warning" onClick={() => takeLoan("30d")}>
                {t('takeLoan30d')} ({Math.round((loan30BP || 0) / 100)}%)
              </button>
            </div>
            <div className="muted">
              {t('example')}: 7д {t('loanTotal')} = {format(calcLoanTotal(clampInt(loanAmount || 0, 0, 500_000_000, 0), loan7BP))} BKC
            </div>
          </div>

          <div className="grid-2 mt-8">
            <button className="btn" onClick={loadBankLoans}>{t('myLoans')}</button>
            <button className="btn btn-secondary" onClick={loadState}>{t('updateBalance')}</button>
          </div>
          {bankLoans?.length ? (
            <div className="wallets mt-8">
              {bankLoans.map((l) => (
                <div key={l.loan_id} className="wallet-item">
                  <div className="wallet-header">
                    <span className="wallet-name">#{l.loan_id} • {l.period}</span>
                    <span className="wallet-address">{format(l.amount)} BKC</span>
                  </div>
                </div>
              ))}
            </div>
          ) : null}
        </section>
      )}

      {tab === "deposit" && (
        <section className="card panel">
          <div className="panel-header">
            <div className="panel-title">{t('deposit')}</div>
            <button className="btn btn-secondary" onClick={loadState}>
              {t('refresh')}
            </button>
          </div>

          <div className="headline">{t('deposit')}</div>
          <div className="muted">{t('rate')}: {format(rate)} BKC = $1</div>

          <div className="headline mt-10">{t('cryptobot')}</div>
          <div className="row">
            <button className="btn btn-primary" onClick={createCryptoPayInvoice}>{t('createInvoice')}</button>
            {invoice?.invoice_id && <button className="btn" onClick={checkCryptoPayInvoice}>{t('check')}</button>}
          </div>
          {invoice?.invoice_id && (
            <div className="address">{t('invoice')} #{invoice.invoice_id} • {invoice.status || "?"} • +{format(invoice.coins)} BKC</div>
          )}

          <div className="headline mt-10">{t('depositToWallet')} ({t('adminConfirmation')})</div>
          {Object.keys(state?.deposit_wallets || {}).length ? (
            <div className="wallets">
              {Object.entries(state?.deposit_wallets || {}).map(([k, v]) => (
                <div key={k} className="wallet-item">
                  <div className="wallet-header">
                    <span className="wallet-name">{k}</span>
                    <button 
                      className="copy-btn"
                      onClick={() => {
                        try {
                          navigator.clipboard.writeText(String(v || ""));
                          setMessage({ type: "info", text: `${k} ${t('addressCopied')}` });
                        } catch {}
                      }}
                    >
                      📋
                    </button>
                  </div>
                  <div className="wallet-address copy" title={t('clickToCopy')}>
                    {v}
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="muted">{t('walletsNotConfigured')}</div>
          )}

          <div className="row">
            <select className="select" value={depositCurrency} onChange={(e) => setDepositCurrency(e.target.value)}>
              <option value="USDT">{t('usdt')}</option>
              <option value="TRX">{t('trx')}</option>
              <option value="SOL">{t('sol')}</option>
            </select>
            <input className="input" type="number" min="1" placeholder={t('sumInUsd')} value={depositUsd} onChange={(e) => setDepositUsd(e.target.value)} />
            <input className="input" placeholder={t('txHash')} value={depositTx} onChange={(e) => setDepositTx(e.target.value)} />
            <button className="btn btn-primary" onClick={createDeposit}>{t('createRequest')}</button>
          </div>

          {state?.is_admin && (
            <>
              {!walletEditorOpen ? (
                <div className="grid-2 mt-8">
                  <button className="btn" onClick={() => setWalletEditorOpen(true)}>{t('editWallets')}</button>
                </div>
              ) : (
                <>
                  <div className="headline mt-10">{t('admin')}: {t('wallets')}</div>
                  {(walletDraft || []).map((r, idx) => (
                    <div className="row" key={idx}>
                      <input className="input" placeholder={t('key') + ' (например USDT_TRC20)'} value={r.k} onChange={(e) => updateWalletRow(idx, "k", e.target.value)} />
                      <input className="input" placeholder={t('address')} value={r.v} onChange={(e) => updateWalletRow(idx, "v", e.target.value)} />
                      <button className="btn btn-danger" onClick={() => removeWalletRow(idx)}>×</button>
                    </div>
                  ))}
                  <div className="grid-2 mt-8">
                    <button className="btn" onClick={addWalletRow}>+ {t('add')}</button>
                    <button className="btn btn-primary" onClick={saveWallets}>{t('save')}</button>
                  </div>
                  <button className="btn mt-8" onClick={() => setWalletEditorOpen(false)}>{t('close')}</button>
                </>
              )}
            </>
          )}

          {state?.is_admin && (
            <>
              <div className="headline mt-10">{t('admin')}: {t('requests')}</div>
              <button className="btn" onClick={adminLoadDeposits}>{t('update')}</button>
              {adminDeposits?.length ? (
                <div className="wallets">
                  {adminDeposits.map((d) => (
                    <div key={d.deposit_id} className="wallet-item">
                      <div className="wallet-header">
                        <span className="wallet-name">#{d.deposit_id} • user {d.user_id} • {d.currency} • ${d.amount_usd} ={'>'} {format(d.coins)} BKC</span>
                      </div>
                      <div className="wallet-address">{d.tx_hash}</div>
                      <div className="grid-2 mt-8">
                        <button className="btn btn-success" onClick={() => adminProcessDeposit(d.deposit_id, true)}>{t('confirm')}</button>
                        <button className="btn btn-danger" onClick={() => adminProcessDeposit(d.deposit_id, false)}>{t('reject')}</button>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="muted">{t('noPendingRequests')}</div>
              )}
            </>
          )}
        </section>
      )}

      {tab === "chain" && (
        <section className="card panel">
          <div className="panel-header">
            <div className="panel-title">{t('blockchain')}</div>
            <button className="btn btn-primary" onClick={loadChain}>{t('updateNetwork')}</button>
          </div>

          {chain && (
            <div className="stats-grid">
              <div className="stat-card">
                <div className="stat-value">{format(chain.users || 0)}</div>
                <div className="stat-label">{t('users')}</div>
              </div>
              <div className="stat-card">
                <div className="stat-value">{format(chain.transactions || 0)}</div>
                <div className="stat-label">{t('transactions')}</div>
              </div>
              <div className="stat-card">
                <div className="stat-value">{format(chain.tapped || 0)}</div>
                <div className="stat-label">{t('tapped')}</div>
              </div>
            </div>
          )}
        </section>
      )}

      {/* Messages */}
      {message && (
        <div className={`message message-${message.type === 'good' ? 'success' : message.type === 'bad' ? 'error' : 'info'}`}>
          {message.text}
        </div>
      )}

      {/* Language Toggle */}
      <div className="text-center mt-16">
        <button className="btn btn-secondary" onClick={toggleLanguage}>
          {language === 'ru' ? '🇬🇧 Switch to English' : '🇷🇺 Переключить на русский'}
        </button>
      </div>

      {/* Bottom Navigation */}
      <nav className="bottom-nav">
        <div className="nav-tabs">
          <button className={`nav-tab ${tab === "tap" ? "active" : ""}`} onClick={() => setTab("tap")}>
            <span className="nav-icon">🪙</span>
            <span className="nav-label">{t('tapTab')}</span>
          </button>
          <button className={`nav-tab ${tab === "wallet" ? "active" : ""}`} onClick={() => setTab("wallet")}>
            <span className="nav-icon">💼</span>
            <span className="nav-label">{t('walletTab')}</span>
          </button>
          <button className={`nav-tab ${tab === "market" ? "active" : ""}`} onClick={() => setTab("market")}>
            <span className="nav-icon">🛒</span>
            <span className="nav-label">{t('marketTab')}</span>
          </button>
          <button className={`nav-tab ${tab === "deposit" ? "active" : ""}`} onClick={() => setTab("deposit")}>
            <span className="nav-icon">💰</span>
            <span className="nav-label">{t('depositTab')}</span>
          </button>
          <button className={`nav-tab ${tab === "chain" ? "active" : ""}`} onClick={() => setTab("chain")}>
            <span className="nav-icon">⛓️</span>
            <span className="nav-label">{t('chainTab')}</span>
          </button>
        </div>
      </nav>
    </main>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />)
