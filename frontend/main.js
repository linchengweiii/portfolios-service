const $ = (sel, root=document) => root.querySelector(sel);
const $$ = (sel, root=document) => Array.from(root.querySelectorAll(sel));

const inferredBase = (location.origin && location.origin.startsWith('http') && location.pathname.startsWith('/app')) ? location.origin : 'http://localhost:8080';
const state = {
  baseUrl: localStorage.getItem('pf_base_url') || inferredBase,
  pfId: null,
};

function setStatus(msg){ $('#status').textContent = msg || ''; }
function fmt(n){ if(n===undefined||n===null) return ''; return Number(n).toLocaleString(undefined,{maximumFractionDigits:2}); }
function fmtPct(n){ if(n===undefined||n===null) return ''; return Number(n).toFixed(2)+'%'; }
function clsPL(n){ return Number(n) > 0 ? 'pos' : 'neg'; }

async function api(path){
  const url = state.baseUrl.replace(/\/$/,'') + path;
  const res = await fetch(url);
  if(!res.ok){ throw new Error(`HTTP ${res.status}`); }
  return res.json();
}

function cardsFromSummary(s){
  const dailyPct = (s.daily_pl_percent!==undefined && s.daily_pl_percent!==null)
    ? `<span class="${clsPL(s.daily_pl_percent)}">${fmtPct(s.daily_pl_percent)}</span>`
    : '';
  const unrealPct = (s.total_unrealized_pl_percent!==undefined && s.total_unrealized_pl_percent!==null)
    ? `<span class="${clsPL(s.total_unrealized_pl_percent)}">${fmtPct(s.total_unrealized_pl_percent)}</span>`
    : '';
  const rows = [
    ['Total Market Value', fmt(s.total_market_value)],
    ...(s.daily_pl!==undefined ? [['Daily P/L (Daily %)', `${fmt(s.daily_pl)} (${dailyPct})`]] : []),
    ['P/L (P/L % peak)', `${fmt(s.total_unrealized_pl)} (${unrealPct})`],
    ['Total Invested', fmt(s.total_invested)],
    ['Balance', fmt(s.balance)],
  ];
  return rows
    .map(([k,v])=> `<div class="card"><span class="k">${k}</span><span class="v">${v}</span></div>`)
    .join('');
}

function tableAlloc(resp){
  const items = resp.items||[];
  const sorted = items.slice().sort((a,b)=> (Number(b.weight_percent)||0) - (Number(a.weight_percent)||0));
  const head = `<tr>
    <th>Symbol</th>
    <th>Weight %</th>
    <th>P/L (invested %)</th>
    <th>Shares</th>
    <th>Invested/Share</th>
    <th>Market/Share</th>
  </tr>`;
  const rows = sorted.map(it=>{
    const shares = Number(it.shares)||0;
    const invested = Number(it.invested)||0;
    const mv = Number(it.market_value)||0;
    const pl = (mv>0 || invested>0) ? (mv - invested) : 0;
    const plPct = invested>0 ? (pl / invested * 100) : null;
    const invPerShare = (shares>0) ? invested / shares : null;
    const mvPerShare = (shares>0 && mv>0) ? mv / shares : null;
    const plPctHtml = (plPct===null) ? '' : `<span class="${clsPL(plPct)}">${fmtPct(plPct)}</span>`;
    return `
      <tr data-symbol="${it.symbol}">
        <td>${it.symbol}</td>
        <td>${fmtPct(it.weight_percent)}</td>
        <td>${plPctHtml}</td>
        <td>${fmt(shares)}</td>
        <td>${invPerShare===null? '': fmt(invPerShare)}</td>
        <td>${mvPerShare===null? '': fmt(mvPerShare)}</td>
      </tr>`;
  }).join('');
  return `<div class="table-wrap"><table><thead>${head}</thead><tbody>${rows}</tbody></table></div>`;
}

async function loadGlobal(){
  setStatus('Loading global...');
  const [summary, alloc] = await Promise.all([
    api('/summary'),
    api('/allocations?basis='+encodeURIComponent($('#globalAllocBasis').value))
  ]);
  $('#globalSummary').innerHTML = cardsFromSummary(summary);
  $('#globalBar').innerHTML = renderStackBar(alloc.items||[], 'global');
  $('#globalAlloc').innerHTML = tableAlloc(alloc);
  bindStackBar('global');
  setStatus('OK');
}

async function loadPortfolios(){
  const pfs = await api('/portfolios');
  const sel = $('#portfolioSelect');
  sel.innerHTML = '';
  pfs.forEach(p=>{
    const opt = document.createElement('option');
    opt.value = p.id; opt.textContent = `${p.name} (${p.base_ccy})`;
    sel.appendChild(opt);
  });
  if(pfs.length){
    state.pfId = pfs[0].id; sel.value = state.pfId;
  }
}

async function loadPortfolio(){
  if(!state.pfId) return;
  setStatus('Loading portfolio...');
  const [summary, alloc] = await Promise.all([
    api(`/portfolios/${state.pfId}/summary`),
    api(`/portfolios/${state.pfId}/allocations?basis=${encodeURIComponent($('#pfAllocBasis').value)}`)
  ]);
  $('#pfSummary').innerHTML = cardsFromSummary(summary);
  $('#pfBar').innerHTML = renderStackBar(alloc.items||[], 'pf');
  $('#pfAlloc').innerHTML = tableAlloc(alloc);
  bindStackBar('pf');
  setStatus('OK');
}

function renderStackBar(items, scope){
  if(!items.length) return '<div class="muted">No positions</div>';
  // colors palette
  const colors = ['#5b9bff','#8bd450','#f39c12','#e74c3c','#9b59b6','#1abc9c','#e67e22','#2ecc71','#ff6b6b','#60a5fa','#f472b6'];
  const sorted = items.slice().sort((a,b)=> (b.weight_percent||0) - (a.weight_percent||0));
  const totalPct = sorted.reduce((a,b)=>a+(b.weight_percent||0),0) || 100;
  const segs = sorted.map((it,i)=>{
    const pct = Math.max(0, it.weight_percent||0) * 100 / totalPct; // normalize in case
    const w = pct.toFixed(2);
    const c = colors[i % colors.length];
    const label = `${it.symbol} ${pct.toFixed(1)}%`;
    const showInline = pct >= 8; // show inline label only if wide enough
    const inner = showInline ? `<span class="segtext">${label}</span>` : '';
    return `<div class="seg" data-scope="${scope}" data-symbol="${it.symbol}" title="${label}" data-label="${label}" style="background:${c};width:${w}%">${inner}</div>`;
  }).join('');
  return `<div class="stackbar">${segs}</div>`;
}

function bindStackBar(scope){
  const barId = scope === 'global' ? '#globalBar' : '#pfBar';
  const tableId = scope === 'global' ? '#globalAlloc' : '#pfAlloc';
  const wrap = $(barId);
  wrap.onclick = (e)=>{
    const seg = e.target.closest('.seg');
    if(!seg) return;
    const symbol = seg.getAttribute('data-symbol');
    // Toggle active
    $$('.seg', wrap).forEach(el=> el.classList.toggle('active', el===seg));
    // Highlight table row
    const table = $(tableId);
    $$('tr', table).forEach(tr=>{
      const sym = tr.getAttribute('data-symbol');
      tr.classList.toggle('hl', sym===symbol);
    });
  };
}

function renderBTResult(targetSel, data){
  const rows = [
    ['Alt P/L (peak %)', `${fmt(data.alt_pl)} (${fmtPct(data.alt_pl_percent)})`],
    ['Current P/L (peak %)', `${fmt(data.current_pl)} (${fmtPct(data.current_pl_percent)})`],
  ];
  $(targetSel).innerHTML = rows.map(([k,v])=>`<div class="card"><span class="k">${k}</span><span class="v">${v}</span></div>`).join('');
}

function wire(){
  $('#baseUrl').value = state.baseUrl;
  $('#baseUrl').addEventListener('change', ()=>{
    state.baseUrl = $('#baseUrl').value.trim().replace(/\/$/,'');
    localStorage.setItem('pf_base_url', state.baseUrl);
  });
  $('#globalAllocBasis').addEventListener('change', loadGlobal);
  $('#pfAllocBasis').addEventListener('change', loadPortfolio);
  $('#portfolioSelect').addEventListener('change', ()=>{ state.pfId = $('#portfolioSelect').value; loadPortfolio(); });
  $('#refreshGlobal').addEventListener('click', loadGlobal);
  $('#refreshPortfolio').addEventListener('click', loadPortfolio);
  $('#refreshAll').addEventListener('click', async ()=>{ await loadGlobal(); await loadPortfolio(); });

  $('#runGlobalBT').addEventListener('click', async ()=>{
    try{
      setStatus('Backtesting (global)...');
      const sym = $('#globalBTsymbol').value.trim();
      if(!sym){ alert('Enter symbol'); return; }
      const ccy = $('#globalBTccy').value.trim()||'USD';
      const basis = $('#globalBTbasis').value;
      const data = await api(`/backtest?symbol=${encodeURIComponent(sym)}&symbol_ccy=${encodeURIComponent(ccy)}&price_basis=${encodeURIComponent(basis)}`);
      renderBTResult('#globalBTResult', data);
      setStatus('OK');
    }catch(e){ setStatus('Backtest error: '+e.message); }
  });

  $('#runPfBT').addEventListener('click', async ()=>{
    try{
      setStatus('Backtesting (portfolio)...');
      const sym = $('#pfBTsymbol').value.trim();
      if(!sym){ alert('Enter symbol'); return; }
      const ccy = $('#pfBTccy').value.trim()||'USD';
      const basis = $('#pfBTbasis').value;
      const data = await api(`/portfolios/${state.pfId}/backtest?symbol=${encodeURIComponent(sym)}&symbol_ccy=${encodeURIComponent(ccy)}&price_basis=${encodeURIComponent(basis)}`);
      renderBTResult('#pfBTResult', data);
      setStatus('OK');
    }catch(e){ setStatus('Backtest error: '+e.message); }
  });
}

(async function init(){
  try{
    wire();
    await loadPortfolios();
    await loadGlobal();
    await loadPortfolio();
  }catch(e){ setStatus('Error: '+e.message); console.error(e); }
})();
