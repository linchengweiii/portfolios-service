const $ = (sel, root=document) => root.querySelector(sel);
const $$ = (sel, root=document) => Array.from(root.querySelectorAll(sel));

const inferredBase = (location.origin && location.origin.startsWith('http') && location.pathname.startsWith('/app')) ? location.origin : 'http://localhost:8080';
const state = {
  baseUrl: localStorage.getItem('pf_base_url') || inferredBase,
  pfId: 'ALL',
  portfolios: [],
  pfNameById: {},
};

function setStatus(msg){ $('#status').textContent = msg || ''; }
function fmt(n){ if(n===undefined||n===null) return ''; return Number(n).toLocaleString(undefined,{maximumFractionDigits:2}); }

async function apiGlobal(path){
  const url = state.baseUrl.replace(/\/$/,'') + path;
  const res = await fetch(url);
  if(!res.ok){ throw new Error(`HTTP ${res.status}`); }
  return res.json();
}

function fmtDateOnly(s){
  if(!s) return '';
  try{
    const d = new Date(s);
    if(Number.isNaN(d.getTime())) return String(s);
    const y = d.getFullYear();
    const m = String(d.getMonth()+1).padStart(2,'0');
    const day = String(d.getDate()).padStart(2,'0');
    return `${y}-${m}-${day}`;
  }catch(_){ return String(s); }
}

function renderTxTable(items, includePortfolio){
  if(!items || !items.length){
    return '<div class="muted">No transactions</div>';
  }
  const head = `<tr>
    ${includePortfolio? '<th>Portfolio</th>': ''}
    <th>Date</th>
    <th>Type</th>
    <th>Symbol</th>
    <th>CCY</th>
    <th>Shares</th>
    <th>Price</th>
    <th>Fee</th>
    <th>Total</th>
  </tr>`;
  const rows = items.map(it=>{
    const pfName = includePortfolio ? (state.pfNameById[it.portfolio_id] || it.portfolio_id || '') : '';
    return `<tr>
      ${includePortfolio? `<td>${pfName}</td>`: ''}
      <td>${fmtDateOnly(it.date)}</td>
      <td>${it.trade_type}</td>
      <td>${it.symbol||''}</td>
      <td>${it.currency||''}</td>
      <td>${fmt(it.shares)}</td>
      <td>${fmt(it.price)}</td>
      <td>${fmt(it.fee)}</td>
      <td>${fmt(it.total)}</td>
    </tr>`;
  }).join('');
  return `<div class="table-wrap"><table><thead>${head}</thead><tbody>${rows}</tbody></table></div>`;
}

function todayYMD(){
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth()+1).padStart(2,'0');
  const day = String(d.getDate()).padStart(2,'0');
  return `${y}/${m}/${day}`;
}

async function loadPortfolios(){
  const pfs = await apiGlobal('/portfolios');
  state.portfolios = Array.isArray(pfs)? pfs : [];
  state.pfNameById = Object.fromEntries(state.portfolios.map(p=>[p.id, p.name]));
  const sel = $('#portfolioSelect');
  sel.innerHTML = '';
  const optAll = document.createElement('option');
  optAll.value = 'ALL'; optAll.textContent = 'ALL';
  sel.appendChild(optAll);
  state.portfolios.forEach(p=>{
    const opt = document.createElement('option');
    opt.value = p.id;
    opt.textContent = String(p.name || '').trim() || p.id;
    sel.appendChild(opt);
  });
  state.pfId = 'ALL';
  sel.value = 'ALL';
}

async function loadTransactions(){
  try{
    setStatus('Loading transactions...');
    const sort = ($('#txSort')?.value)||'date_desc';
    const symbol = ($('#txSymbol')?.value||'').trim();
    const limit = Number(($('#txLimit')?.value)||'50');
    let items = [];
    if(state.pfId === 'ALL'){
      const pfs = state.portfolios.length ? state.portfolios : await apiGlobal('/portfolios');
      if(!state.portfolios.length){
        state.portfolios = pfs; state.pfNameById = Object.fromEntries(pfs.map(p=>[p.id,p.name]));
      }
      const qs = [];
      if(symbol) qs.push('symbol='+encodeURIComponent(symbol));
      if(sort) qs.push('sort='+encodeURIComponent(sort));
      if(Number.isFinite(limit) && limit>0) qs.push('limit='+limit);
      const query = qs.length? ('?'+qs.join('&')) : '';
      const chunks = await Promise.all(pfs.map(async p=>{
        const url = state.baseUrl.replace(/\/$/,'') + `/portfolios/${p.id}/transactions${query}`;
        const res = await fetch(url);
        if(!res.ok) return [];
        const arr = await res.json();
        return Array.isArray(arr)? arr.map(x=> ({...x, portfolio_id: p.id})) : [];
      }));
      items = chunks.flat();
      const toTime = (s)=>{ const d=new Date(s); return d.getTime()||0; };
      items.sort((a,b)=> sort==='date_asc' ? (toTime(a.date)-toTime(b.date)) : (toTime(b.date)-toTime(a.date)) );
      if(Number.isFinite(limit) && limit>0){ items = items.slice(0, limit); }
      $('#txList').innerHTML = renderTxTable(items, true);
    } else {
      const qs = [];
      if(symbol) qs.push('symbol='+encodeURIComponent(symbol));
      if(sort) qs.push('sort='+encodeURIComponent(sort));
      if(Number.isFinite(limit) && limit>0) qs.push('limit='+limit);
      const query = qs.length? ('?'+qs.join('&')) : '';
      const url = state.baseUrl.replace(/\/$/,'') + `/portfolios/${state.pfId}/transactions${query}`;
      const res = await fetch(url);
      if(!res.ok){ throw new Error(`HTTP ${res.status}`); }
      items = await res.json();
      $('#txList').innerHTML = renderTxTable(items, false);
    }
    setStatus('OK');
  }catch(e){
    $('#txList').innerHTML = `<div class="muted">Error loading transactions: ${e.message}</div>`;
    setStatus('Error');
  }
}

function wire(){
  $('#baseUrl').value = state.baseUrl;
  $('#baseUrl').addEventListener('change', ()=>{
    state.baseUrl = $('#baseUrl').value.trim().replace(/\/$/,'');
    localStorage.setItem('pf_base_url', state.baseUrl);
  });
  $('#portfolioSelect').addEventListener('change', async ()=>{
    state.pfId = $('#portfolioSelect').value;
    toggleAddPanel();
    await loadTransactions();
  });
  $('#txReload')?.addEventListener('click', loadTransactions);
  $('#txSort')?.addEventListener('change', loadTransactions);
  $('#txLimit')?.addEventListener('change', loadTransactions);

  // Add form defaults and behavior
  const dateInput = $('#newDate');
  if(dateInput && !dateInput.value){ dateInput.value = todayYMD(); }
  const typeSel = $('#newType');
  const symInput = $('#newSymbol');
  const sharesInput = $('#newShares');
  const priceInput = $('#newPrice');
  const feeInput = $('#newFee');
  const totalInput = $('#newTotal');
  const ccyInput = $('#newCurrency');

  const toggleByType = ()=>{
    const tt = (typeSel?.value||'').toLowerCase();
    const isCash = tt === 'cash';
    [symInput, sharesInput, priceInput, feeInput].forEach(el=>{ if(!el) return; el.disabled = isCash; });
    if(isCash){
      if(symInput) symInput.value = '';
      if(sharesInput) sharesInput.value = '';
      if(priceInput) priceInput.value = '';
      if(feeInput) feeInput.value = '0';
    }
  };
  typeSel?.addEventListener('change', toggleByType);
  toggleByType();

  $('#addTx')?.addEventListener('click', async ()=>{
    try{
      if(state.pfId === 'ALL'){ alert('Please select a specific portfolio to add a transaction.'); return; }
      const trade_type = (typeSel?.value||'').toLowerCase();
      const symbol = (symInput?.value||'').trim();
      const currency = (ccyInput?.value||'').trim().toUpperCase() || 'USD';
      const shares = Number(sharesInput?.value||'0');
      const price = Number(priceInput?.value||'0');
      const fee = Number(feeInput?.value||'0');
      const date = (dateInput?.value||'').trim();
      const total = Number(totalInput?.value||'0');
      if(!date){ alert('Date is required (YYYY/MM/DD)'); return; }
      if(trade_type !== 'cash' && !symbol){ alert('Symbol is required for non-cash transactions'); return; }
      // For safety, ensure total is not NaN
      if(!Number.isFinite(total)){ alert('Total must be a number'); return; }
      const payload = { trade_type, currency, date, total };
      if(trade_type !== 'cash'){
        payload.symbol = symbol.toUpperCase();
        payload.shares = shares;
        payload.price = price;
        payload.fee = fee;
      }
      setStatus('Adding transaction...');
      const url = state.baseUrl.replace(/\/$/,'') + `/portfolios/${state.pfId}/transactions`;
      const res = await fetch(url, { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify(payload) });
      if(!res.ok){ const t = await res.text(); throw new Error(`HTTP ${res.status}: ${t}`); }
      // Clear inputs (keep date, currency, type)
      if(symInput) symInput.value=''; if(sharesInput) sharesInput.value=''; if(priceInput) priceInput.value=''; if(feeInput) feeInput.value='0'; if(totalInput) totalInput.value='';
      await loadTransactions();
      setStatus('OK');
    }catch(e){ setStatus('Add error: '+e.message); }
  });
}

(async function init(){
  try{
    wire();
    await loadPortfolios();
    toggleAddPanel();
    await loadTransactions();
  }catch(e){ setStatus('Error: '+e.message); console.error(e); }
})();

function toggleAddPanel(){
  const panel = document.getElementById('addPanel');
  if(!panel) return;
  const isAll = state.pfId === 'ALL';
  panel.classList.toggle('hidden', isAll);
}
