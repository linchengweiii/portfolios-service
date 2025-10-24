// Mobile build: mostly identical to desktop main.js with base inference for /mobile
const $ = (sel, root=document) => root.querySelector(sel);
const $$ = (sel, root=document) => Array.from(root.querySelectorAll(sel));

const inferredBase = (location.origin && location.origin.startsWith('http') && (location.pathname.startsWith('/mobile') || location.pathname.startsWith('/app'))) ? location.origin : 'http://localhost:8080';
const state = {
  baseUrl: localStorage.getItem('pf_base_url') || inferredBase,
  globalRefCcy: localStorage.getItem('pf_ref_ccy_global') || 'TWD',
  pfRefCcyMap: JSON.parse(localStorage.getItem('pf_ref_ccy_map') || '{}'),
  pfId: 'ALL',
  labelThreshold: Number(localStorage.getItem('pf_label_threshold') || '5'),
  portfolios: [],
  pfNameById: {},
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
  const equityTotal = Number(s.total_market_value||0) + Number(s.balance||0);
  const dailyPLVal = (s.daily_pl===undefined || s.daily_pl===null) ? null : Number(s.daily_pl);
  const dailyPLHtml = (dailyPLVal===null) ? '' : (dailyPLVal===0 ? fmt(dailyPLVal) : `<span class="${clsPL(dailyPLVal)}">${fmt(dailyPLVal)}</span>`);
  const dailyPctVal = (s.daily_pl_percent===undefined || s.daily_pl_percent===null) ? null : Number(s.daily_pl_percent);
  const dailyPctHtml = (dailyPctVal===null) ? '' : (dailyPctVal===0 ? fmtPct(dailyPctVal) : `<span class="${clsPL(dailyPctVal)}">${fmtPct(dailyPctVal)}</span>`);
  const plVal = (s.total_unrealized_pl===undefined || s.total_unrealized_pl===null) ? null : Number(s.total_unrealized_pl);
  const plHtml = (plVal===null) ? '' : (plVal===0 ? fmt(plVal) : `<span class="${clsPL(plVal)}">${fmt(plVal)}</span>`);
  const plPctVal = (s.total_unrealized_pl_percent===undefined || s.total_unrealized_pl_percent===null) ? null : Number(s.total_unrealized_pl_percent);
  const plPctHtml = (plPctVal===null) ? '' : (plPctVal===0 ? fmtPct(plPctVal) : `<span class="${clsPL(plPctVal)}">${fmtPct(plPctVal)}</span>`);
  const cost = Number(s.effective_cash_in_peak||0);
  const plSub = `<small class=\"sub muted\">P/L: ${fmt(plVal||0)} • Cost: ${fmt(cost)}${cost>0 ? ` = ${fmtPct((plVal||0)/cost*100)}` : ''}</small>`;
  const rows = [
    ['Total Market Value', `${fmt(equityTotal)}<br><small class=\"sub muted\">Holdings: ${fmt(s.total_market_value)} • Balance: ${fmt(s.balance)}</small>`],
    ...(dailyPLVal===null ? [] : [['Daily P/L (Percentage)', `${dailyPLHtml} (${dailyPctHtml})`]]),
    ['P/L (Percentage)', `${plHtml} (${plPctHtml})<br>${plSub}`],
  ];
  return rows
    .map(([k,v])=> `<div class=\"card\"><span class=\"k\">${k}</span><span class=\"v\">${v}</span></div>`)
    .join('');
}

function tableAlloc(resp){
  const items = resp.items||[];
  const sorted = items.slice().sort((a,b)=> (Number(b.weight_percent)||0) - (Number(a.weight_percent)||0));
  const showDaily = String(resp.basis||'').toLowerCase() === 'market_value';
  const head = `<tr>
    <th>Symbol</th>
    ${showDaily ? '<th>Daily P/L</th>' : ''}
    <th>P/L</th>
  </tr>`;
  let totalWeight = 0, totalInvested = 0, totalMV = 0, totalPL = 0, totalDailyPL = 0, totalPrevMV = 0, totalShares = 0;
  const rows = sorted.map(it=>{
    const shares = Number(it.shares)||0;
    const invested = Number(it.invested)||0;
    const mv = Number(it.market_value)||0;
    const pl = (mv>0 || invested>0) ? (mv - invested) : 0;
    const plPct = invested>0 ? (pl / invested * 100) : null;
    totalWeight += Number(it.weight_percent)||0;
    totalInvested += invested;
    totalMV += mv;
    totalPL += pl;
    totalShares += shares;
    const dailyPL = (it.daily_pl===undefined||it.daily_pl===null) ? null : Number(it.daily_pl);
    const dailyPct = (it.daily_pl_percent===undefined||it.daily_pl_percent===null) ? null : Number(it.daily_pl_percent);
    if(dailyPL!==null) totalDailyPL += dailyPL;
    const prevMV = (it.daily_prev_market_value===undefined||it.daily_prev_market_value===null) ? null : Number(it.daily_prev_market_value);
    if(prevMV!==null) totalPrevMV += prevMV;
    const dailyPctHtml = dailyPct===null? '' : `<span class=\"${clsPL(dailyPct)}\">${fmtPct(dailyPct)}</span>`;
    const dailyAmtApprox = dailyPL===null? '' : fmt(dailyPL);
    const plPctHtml = plPct===null? '' : `<span class=\"${clsPL(plPct)}\">${fmtPct(plPct)}</span>`;
    const plAmtApprox = fmt(pl);
    return `
      <tr data-symbol="${it.symbol}">
        <td class="symcol"><span class="sym">${it.symbol}</span><span class="shares muted">${fmt(shares)} shares</span></td>
        ${showDaily ? `<td>${dailyPctHtml}<br><small class=\"muted\">≈ ${dailyAmtApprox}</small></td>` : ''}
        <td>${plPctHtml}<br><small class=\"muted\">≈ ${plAmtApprox}</small></td>
      </tr>`;
  }).join('');
  const totalPlPct = totalInvested>0 ? (totalPL/totalInvested*100) : null;
  const totalDailyPct = (showDaily && totalPrevMV>0) ? (totalDailyPL/totalPrevMV*100) : null;
  const totalRow = `
    <tr class="total">
      <td>Total</td>
      ${showDaily ? `<td><span class=\"${clsPL(totalDailyPct||0)}\">${totalDailyPct===null? '' : fmtPct(totalDailyPct)}</span><br><small class=\"muted\">≈ ${fmt(totalDailyPL)}</small></td>` : ''}
      <td><span class=\"${clsPL(totalPlPct||0)}\">${totalPlPct===null? '' : fmtPct(totalPlPct)}</span><br><small class=\"muted\">≈ ${fmt(totalPL)}</small></td>
    </tr>`;
  return `<div class=\"table-wrap\"><table><thead>${head}</thead><tbody>${rows}${totalRow}</tbody></table></div>`;
}

async function loadView(){
  const basis = 'market_value';
  if(state.pfId === 'ALL'){
    setStatus('Loading (ALL)...');
    const [summary, alloc] = await Promise.all([
      apiGlobal('/summary'),
      apiGlobal('/allocations?basis='+encodeURIComponent(basis))
    ]);
    $('#summary').innerHTML = cardsFromSummary(summary);
    $('#bar').innerHTML = renderPie(alloc.items||[]);
    $('#alloc').innerHTML = tableAlloc(alloc);
    bindPie();
    setStatus('OK');
  } else {
    setStatus('Loading portfolio...');
    const [summary, alloc] = await Promise.all([
      apiPf(`/portfolios/${state.pfId}/summary`),
      apiPf(`/portfolios/${state.pfId}/allocations?basis=${encodeURIComponent(basis)}`)
    ]);
    $('#summary').innerHTML = cardsFromSummary(summary);
    $('#bar').innerHTML = renderPie(alloc.items||[]);
    $('#alloc').innerHTML = tableAlloc(alloc);
    bindPie();
    setStatus('OK');
  }
}

async function loadPortfolios(){
  const pfs = await apiGlobal('/portfolios');
  const sel = $('#portfolioSelect');
  sel.innerHTML = '';
  state.portfolios = Array.isArray(pfs) ? pfs : [];
  state.pfNameById = Object.fromEntries(state.portfolios.map(p=>[p.id, p.name]));
  const optAll = document.createElement('option');
  optAll.value = 'ALL'; optAll.textContent = 'ALL';
  sel.appendChild(optAll);
  pfs.forEach(p=>{
    const opt = document.createElement('option');
    opt.value = p.id;
    opt.textContent = String(p.name || '').trim() || p.id;
    sel.appendChild(opt);
  });
  state.pfId = 'ALL';
  sel.value = 'ALL';
}

function renderPie(items){
  if(!items.length) return '<div class="muted">No positions</div>';
  const thresholdSel = $('#labelThreshold');
  const threshold = Math.max(0, Math.min(100, Number(thresholdSel ? thresholdSel.value : state.labelThreshold || 5)));
  const colors = ['#5b9bff','#8bd450','#f39c12','#e74c3c','#9b59b6','#1abc9c','#e67e22','#2ecc71','#ff6b6b','#60a5fa','#f472b6'];
  const sorted = items.slice().sort((a,b)=> (Number(b.weight_percent)||0) - (Number(a.weight_percent)||0));
  const totalPct = sorted.reduce((a,b)=>a+(Number(b.weight_percent)||0),0) || 100;
  const majors = [], minors = [];
  sorted.forEach(it => {
    const pct = Math.max(0, Number(it.weight_percent)||0) * 100 / totalPct;
    if(pct < threshold){ minors.push(it); } else { majors.push(it); }
  });
  const minorSum = minors.reduce((a,b)=> a + (Number(b.weight_percent)||0), 0);
  const itemsForPie = majors.slice();
  if(minorSum > 0){ itemsForPie.push({ symbol: 'OTHERS', weight_percent: minorSum }); }

  const size = 220, cx = size/2, cy = size/2, r = size*0.4;
  let angle = -Math.PI/2; // start at top
  const arcs = itemsForPie.map((it,i)=>{
    const pct = Math.max(0, Number(it.weight_percent)||0) * 100 / totalPct;
    const theta = (pct/100) * Math.PI*2;
    const x1 = cx + r * Math.cos(angle);
    const y1 = cy + r * Math.sin(angle);
    const end = angle + theta;
    const x2 = cx + r * Math.cos(end);
    const y2 = cy + r * Math.sin(end);
    const large = theta > Math.PI ? 1 : 0;
    angle = end;
    const c = colors[i % colors.length];
    const label = `${it.symbol} ${pct.toFixed(1)}%`;
    const d = `M ${cx} ${cy} L ${x1} ${y1} A ${r} ${r} 0 ${large} 1 ${x2} ${y2} Z`;
    return `<path class=\"slice\" data-symbol=\"${it.symbol}\" data-color=\"${c}\" data-pct=\"${pct.toFixed(1)}\" d=\"${d}\" fill=\"${c}\" title=\"${label}\"/>`;
  }).join('');
  const donut = `<circle cx=\"${cx}\" cy=\"${cy}\" r=\"${r*0.55}\" fill=\"#0b0d14\" stroke=\"var(--border)\"/>`;
  const legend = itemsForPie.map((it,i)=>{
    const pct = Math.max(0, Number(it.weight_percent)||0) * 100 / totalPct;
    const c = colors[i % colors.length];
    return `<div class=\"legend-item\" data-symbol=\"${it.symbol}\"><span class=\"swatch\" style=\"background:${c}\"></span><span class=\"name\">${it.symbol}</span><span class=\"pct muted\">${pct.toFixed(1)}%</span></div>`;
  }).join('');
  return `<div class=\"pie-block\"><svg class=\"pie\" viewBox=\"0 0 ${size} ${size}\" role=\"img\">${arcs}${donut}</svg><div class=\"legend\">${legend}</div></div>`;
}

function bindPie(){
  const wrap = $('#bar');
  wrap.onclick = (e)=>{
    const slice = e.target.closest('.slice');
    const item = e.target.closest('.legend-item');
    const symbol = slice ? slice.getAttribute('data-symbol') : (item ? item.getAttribute('data-symbol') : null);
    if(!symbol) return;
    // highlight slice
    const targetSlice = $(`.slice[data-symbol=\"${symbol}\"]`, wrap);
    $$('.slice', wrap).forEach(el=> el.classList.toggle('active', el===targetSlice));
    // highlight legend item
    const targetLegend = $(`.legend-item[data-symbol=\"${symbol}\"]`, wrap);
    $$('.legend-item', wrap).forEach(el=> el.classList.toggle('active', el===targetLegend));
    // highlight table row
    const table = $('#alloc');
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
    ['Alt Max Drop', `${fmtPct(data.alt_max_drop_percent)}`],
    ['Current Max Drop', `${fmtPct(data.current_max_drop_percent)}`],
  ];
  $(targetSel).innerHTML = rows.map(([k,v])=>`<div class=\"card\"><span class=\"k\">${k}</span><span class=\"v\">${v}</span></div>`).join('');
}

function wire(){
  $('#baseUrl').value = state.baseUrl;
  $('#baseUrl').addEventListener('change', ()=>{
    state.baseUrl = $('#baseUrl').value.trim().replace(/\/$/,'');
    localStorage.setItem('pf_base_url', state.baseUrl);
  });
  const lt = $('#labelThreshold');
  if(lt){
    lt.innerHTML = '';
    for(let i=0;i<=100;i++){
      const opt = document.createElement('option');
      opt.value = String(i);
      opt.textContent = `${i}%`;
      lt.appendChild(opt);
    }
    if(!Number.isFinite(state.labelThreshold)) state.labelThreshold = 5;
    lt.value = String(Math.max(0, Math.min(100, state.labelThreshold)));
    lt.addEventListener('change', ()=>{
      state.labelThreshold = Number(lt.value);
      localStorage.setItem('pf_label_threshold', String(state.labelThreshold));
      loadView();
    });
  }
  $('#refCcy').addEventListener('change', async ()=>{
    const value = $('#refCcy').value;
    if(state.pfId === 'ALL'){
      state.globalRefCcy = value;
      localStorage.setItem('pf_ref_ccy_global', state.globalRefCcy);
    } else {
      state.pfRefCcyMap[state.pfId] = value;
      localStorage.setItem('pf_ref_ccy_map', JSON.stringify(state.pfRefCcyMap));
    }
    await loadView();
  });
  // no alloc basis selector on mobile
  $('#portfolioSelect').addEventListener('change', async ()=>{
    state.pfId = $('#portfolioSelect').value;
    if(state.pfId === 'ALL'){
      $('#refCcy').value = state.globalRefCcy;
    } else {
      const stored = state.pfRefCcyMap[state.pfId];
      if(stored){
        $('#refCcy').value = stored;
      } else {
        const inferred = await inferPfRefCcy(state.pfId);
        const pick = inferred || 'TWD';
        state.pfRefCcyMap[state.pfId] = pick;
        localStorage.setItem('pf_ref_ccy_map', JSON.stringify(state.pfRefCcyMap));
        $('#refCcy').value = pick;
      }
    }
    await loadView();
  });
  $('#refresh').addEventListener('click', loadView);

  $('#runBT').addEventListener('click', async ()=>{
    try{
      setStatus('Backtesting...');
      const sym = $('#btSymbol').value.trim();
      if(!sym){ alert('Enter symbol'); return; }
      const ccy = $('#btCcy').value.trim()||'USD';
      const basis = $('#btBasis').value;
      if(state.pfId === 'ALL'){
        const data = await apiGlobal(`/backtest?symbol=${encodeURIComponent(sym)}&symbol_ccy=${encodeURIComponent(ccy)}&price_basis=${encodeURIComponent(basis)}`);
        renderBTResult('#btResult', data);
      } else {
        const data = await apiPf(`/portfolios/${state.pfId}/backtest?symbol=${encodeURIComponent(sym)}&symbol_ccy=${encodeURIComponent(ccy)}&price_basis=${encodeURIComponent(basis)}`);
        renderBTResult('#btResult', data);
      }
      setStatus('OK');
    }catch(e){ setStatus('Backtest error: '+e.message); }
  });
}

async function inferPfRefCcy(pfId){
  try{
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
    $('#refCcy').value = state.globalRefCcy;
    await loadView();
  }catch(e){ setStatus('Error: '+e.message); console.error(e); }
})();
