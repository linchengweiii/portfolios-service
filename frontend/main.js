const $ = (sel, root=document) => root.querySelector(sel);
const $$ = (sel, root=document) => Array.from(root.querySelectorAll(sel));

const inferredBase = (location.origin && location.origin.startsWith('http') && location.pathname.startsWith('/app')) ? location.origin : 'http://localhost:8080';
const state = {
  baseUrl: localStorage.getItem('pf_base_url') || inferredBase,
  globalRefCcy: localStorage.getItem('pf_ref_ccy_global') || 'TWD',
  pfRefCcyMap: JSON.parse(localStorage.getItem('pf_ref_ccy_map') || '{}'),
  pfId: null,
};

function setStatus(msg){ $('#status').textContent = msg || ''; }
function fmt(n){ if(n===undefined||n===null) return ''; return Number(n).toLocaleString(undefined,{maximumFractionDigits:2}); }
function fmtPct(n){ if(n===undefined||n===null) return ''; return Number(n).toFixed(2)+'%'; }
function clsPL(n){ return Number(n) > 0 ? 'pos' : 'neg'; }

function withRef(path, ref){
  if(!ref) return path;
  const sep = path.includes('?') ? '&' : '?';
  return `${path}${sep}ref_ccy=${encodeURIComponent(ref)}`;
}

async function apiGlobal(path){
  const url = state.baseUrl.replace(/\/$/,'') + withRef(path, state.globalRefCcy);
  const res = await fetch(url);
  if(!res.ok){ throw new Error(`HTTP ${res.status}`); }
  return res.json();
}

async function apiPf(path){
  const ref = state.pfRefCcyMap[state.pfId] || state.globalRefCcy || 'TWD';
  const url = state.baseUrl.replace(/\/$/,'') + withRef(path, ref);
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
  const showDaily = String(resp.basis||'').toLowerCase() === 'market_value';
  const showPLAmount = showDaily; // show P/L amount when we have market values
  const head = `<tr>
    <th>Symbol</th>
    <th>Weight %</th>
    <th>P/L (invested %)</th>
    ${showDaily ? '<th>Daily P/L (Daily %)</th>' : ''}
    <th>Shares</th>
    <th>Invested/Share</th>
    <th>Market/Share</th>
  </tr>`;
  let totalWeight = 0, totalInvested = 0, totalMV = 0, totalPL = 0, totalDailyPL = 0, totalPrevMV = 0, totalShares = 0;
  const rows = sorted.map(it=>{
    const shares = Number(it.shares)||0;
    const invested = Number(it.invested)||0;
    const mv = Number(it.market_value)||0;
    const pl = (mv>0 || invested>0) ? (mv - invested) : 0;
    const plPct = invested>0 ? (pl / invested * 100) : null;
    const invPerShare = (shares>0) ? invested / shares : null;
    const mvPerShare = (shares>0 && mv>0) ? mv / shares : null;
    const plPctHtml = (plPct===null) ? '' : `<span class="${clsPL(plPct)}">${fmtPct(plPct)}</span>`;
    const plHtml = showPLAmount
      ? `${fmt(pl)}${plPct===null? '' : ` (${plPctHtml})`}`
      : `${plPctHtml}`;
    const dailyPL = (it.daily_pl===undefined||it.daily_pl===null) ? null : Number(it.daily_pl);
    const dailyPct = (it.daily_pl_percent===undefined||it.daily_pl_percent===null) ? null : Number(it.daily_pl_percent);
    const dailyHtml = (!showDaily) ? '' : `<td>${dailyPL===null? '' : fmt(dailyPL)}${dailyPct===null? '' : ` (<span class="${clsPL(dailyPct)}">${fmtPct(dailyPct)}</span>)`}</td>`;
    // Accumulate totals
    totalWeight += Number(it.weight_percent)||0;
    totalInvested += invested;
    totalMV += mv;
    totalPL += pl;
    totalShares += shares;
    if (dailyPL!==null) totalDailyPL += dailyPL;
    const prevMV = (it.daily_prev_market_value===undefined||it.daily_prev_market_value===null) ? null : Number(it.daily_prev_market_value);
    if (prevMV!==null) totalPrevMV += prevMV;
    return `
      <tr data-symbol="${it.symbol}">
        <td>${it.symbol}</td>
        <td>${fmtPct(it.weight_percent)}</td>
        <td>${plHtml}</td>
        ${dailyHtml}
        <td>${fmt(shares)}</td>
        <td>${invPerShare===null? '': fmt(invPerShare)}</td>
        <td>${mvPerShare===null? '': fmt(mvPerShare)}</td>
      </tr>`;
  }).join('');
  // Totals row
  const totalPlPct = totalInvested>0 ? (totalPL/totalInvested*100) : null;
  const totalPlHtml = showPLAmount
    ? `${fmt(totalPL)}${totalPlPct===null? '' : ` (<span class="${clsPL(totalPlPct)}">${fmtPct(totalPlPct)}</span>)`}`
    : `${totalPlPct===null? '' : `<span class="${clsPL(totalPlPct)}">${fmtPct(totalPlPct)}</span>`}`;
  const totalDailyPct = (showDaily && totalPrevMV>0) ? (totalDailyPL/totalPrevMV*100) : null;
  const totalDailyHtml = (!showDaily) ? '' : `<td>${fmt(totalDailyPL)}${totalDailyPct===null? '' : ` (<span class="${clsPL(totalDailyPct)}">${fmtPct(totalDailyPct)}</span>)`}</td>`;
  const totalRow = `
    <tr class="total">
      <td>Total</td>
      <td>${fmtPct(totalWeight)}</td>
      <td>${totalPlHtml}</td>
      ${totalDailyHtml}
      <td>${fmt(totalShares)}</td>
      <td></td>
      <td></td>
    </tr>`;
  return `<div class="table-wrap"><table><thead>${head}</thead><tbody>${rows}${totalRow}</tbody></table></div>`;
}

async function loadGlobal(){
  setStatus('Loading global...');
  const [summary, alloc] = await Promise.all([
    apiGlobal('/summary'),
    apiGlobal('/allocations?basis='+encodeURIComponent($('#globalAllocBasis').value))
  ]);
  $('#globalSummary').innerHTML = cardsFromSummary(summary);
  $('#globalBar').innerHTML = renderStackBar(alloc.items||[], 'global');
  $('#globalAlloc').innerHTML = tableAlloc(alloc);
  bindStackBar('global');
  setStatus('OK');
}

async function loadPortfolios(){
  const pfs = await apiGlobal('/portfolios');
  const sel = $('#portfolioSelect');
  sel.innerHTML = '';
  pfs.forEach(p=>{
    const opt = document.createElement('option');
    const base = (p.base_ccy && String(p.base_ccy).trim()) ? ` (${p.base_ccy})` : '';
    opt.value = p.id; opt.textContent = `${p.name}${base}`;
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
    apiPf(`/portfolios/${state.pfId}/summary`),
    apiPf(`/portfolios/${state.pfId}/allocations?basis=${encodeURIComponent($('#pfAllocBasis').value)}`)
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
  // Global Ref CCY toggle
  $('#refCcyGlobal').value = state.globalRefCcy;
  $('#refCcyGlobal').addEventListener('change', async ()=>{
    state.globalRefCcy = $('#refCcyGlobal').value;
    localStorage.setItem('pf_ref_ccy_global', state.globalRefCcy);
    await loadGlobal();
  });

  // Portfolio Ref CCY toggle
  $('#refCcyPf').addEventListener('change', async ()=>{
    if(!state.pfId) return;
    state.pfRefCcyMap[state.pfId] = $('#refCcyPf').value;
    localStorage.setItem('pf_ref_ccy_map', JSON.stringify(state.pfRefCcyMap));
    await loadPortfolio();
  });
  $('#globalAllocBasis').addEventListener('change', loadGlobal);
  $('#pfAllocBasis').addEventListener('change', loadPortfolio);
  $('#portfolioSelect').addEventListener('change', async ()=>{
    state.pfId = $('#portfolioSelect').value;
    // Set PF ref CCY: use stored or infer
    const stored = state.pfRefCcyMap[state.pfId];
    if(stored){
      $('#refCcyPf').value = stored;
    }else{
      const inferred = await inferPfRefCcy(state.pfId);
      const pick = inferred || 'TWD';
      state.pfRefCcyMap[state.pfId] = pick;
      localStorage.setItem('pf_ref_ccy_map', JSON.stringify(state.pfRefCcyMap));
      $('#refCcyPf').value = pick;
    }
    await loadPortfolio();
  });
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
      const data = await apiGlobal(`/backtest?symbol=${encodeURIComponent(sym)}&symbol_ccy=${encodeURIComponent(ccy)}&price_basis=${encodeURIComponent(basis)}`);
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
      const data = await apiPf(`/portfolios/${state.pfId}/backtest?symbol=${encodeURIComponent(sym)}&symbol_ccy=${encodeURIComponent(ccy)}&price_basis=${encodeURIComponent(basis)}`);
      renderBTResult('#pfBTResult', data);
      setStatus('OK');
    }catch(e){ setStatus('Backtest error: '+e.message); }
  });
}

async function inferPfRefCcy(pfId){
  try{
    // Fetch all tx for the portfolio (no ref_ccy needed)
    const url = state.baseUrl.replace(/\/$/,'') + `/portfolios/${pfId}/transactions?limit=0`;
    const res = await fetch(url);
    if(!res.ok) return null;
    const txs = await res.json();
    const counts = { TWD: 0, USD: 0 };
    txs.forEach(tx => {
      const t = (tx.trade_type||'').toLowerCase();
      if(t==='buy' || t==='sell' || t==='dividend'){
        const c = String(tx.currency||'').trim().toUpperCase();
        if(c==='TWD' || c==='USD') counts[c]++;
      }
    });
    if(counts.USD > counts.TWD) return 'USD';
    if(counts.TWD > counts.USD) return 'TWD';
    return null;
  }catch(_){ return null; }
}

(async function init(){
  try{
    wire();
    await loadPortfolios();
    // Initialize PF ref CCY for the first portfolio
    if(state.pfId){
      const stored = state.pfRefCcyMap[state.pfId];
      if(stored){
        $('#refCcyPf').value = stored;
      } else {
        const inferred = await inferPfRefCcy(state.pfId);
        const pick = inferred || 'TWD';
        state.pfRefCcyMap[state.pfId] = pick;
        localStorage.setItem('pf_ref_ccy_map', JSON.stringify(state.pfRefCcyMap));
        $('#refCcyPf').value = pick;
      }
    }
    await loadGlobal();
    await loadPortfolio();
  }catch(e){ setStatus('Error: '+e.message); console.error(e); }
})();
