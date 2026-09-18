const state = { apartments: [] };

const apartmentsEl = document.querySelector('#apartments');
const statusEl = document.querySelector('#requestStatus');
const nodeBadge = document.querySelector('#nodeBadge');
const searchForm = document.querySelector('#searchForm');
const modalBackdrop = document.querySelector('#modalBackdrop');
const bookingForm = document.querySelector('#bookingForm');
const bookingResult = document.querySelector('#bookingResult');

function rub(value) {
  return new Intl.NumberFormat('ru-RU').format(value) + ' ₽';
}

async function readJSON(response) {
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error || `HTTP ${response.status}`);
  return body;
}

async function loadNode() {
  try {
    const response = await fetch('/api/node', { cache: 'no-store' });
    const data = await readJSON(response);
    nodeBadge.textContent = `backend: ${data.backend_node}`;
  } catch {
    nodeBadge.textContent = 'backend: недоступен';
  }
}

function searchParams() {
  const params = new URLSearchParams();
  const city = document.querySelector('#city').value.trim();
  const checkIn = document.querySelector('#checkIn').value;
  const checkOut = document.querySelector('#checkOut').value;
  const guests = document.querySelector('#guests').value;

  if (city) params.set('city', city);
  if (checkIn) params.set('check_in', checkIn);
  if (checkOut) params.set('check_out', checkOut);
  params.set('guests', guests || '1');
  return params;
}

async function loadApartments() {
  statusEl.textContent = 'Запрашиваем каталог…';
  apartmentsEl.innerHTML = '';
  try {
    const response = await fetch('/api/apartments?' + searchParams().toString(), { cache: 'no-store' });
    const data = await readJSON(response);
    state.apartments = data.items;
    nodeBadge.textContent = `backend: ${data.backend_node}`;
    statusEl.textContent = `Найдено: ${data.items.length}`;
    renderApartments();
  } catch (err) {
    statusEl.textContent = 'Ошибка загрузки';
    apartmentsEl.innerHTML = `<div class="booking-result error">${escapeHTML(err.message)}</div>`;
  }
}

function renderApartments() {
  if (!state.apartments.length) {
    apartmentsEl.innerHTML = '<div class="booking-result">На выбранные параметры свободных квартир нет.</div>';
    return;
  }

  apartmentsEl.innerHTML = state.apartments.map(a => `
    <article class="card">
      <div class="card-visual ${escapeHTML(a.accent)}">
        <div class="rating">★ ${escapeHTML(a.rating)}</div>
      </div>
      <div class="card-body">
        <div class="card-title">${escapeHTML(a.title)}</div>
        <div class="card-address">${escapeHTML(a.city)} · ${escapeHTML(a.address)}</div>
        <div class="card-desc">${escapeHTML(a.description)}</div>
        <div class="meta"><span>${a.rooms} комн.</span><span>до ${a.guests} гостей</span></div>
        <div class="card-foot">
          <div class="price"><strong>${rub(a.price_night)}</strong><div class="muted">за ночь</div></div>
          <button type="button" data-book="${a.id}">Выбрать</button>
        </div>
      </div>
    </article>
  `).join('');

  document.querySelectorAll('[data-book]').forEach(btn => {
    btn.addEventListener('click', () => openBooking(Number(btn.dataset.book)));
  });
}

function openBooking(id) {
  const apartment = state.apartments.find(a => a.id === id);
  if (!apartment) return;

  document.querySelector('#apartmentId').value = apartment.id;
  document.querySelector('#modalTitle').textContent = apartment.title;
  document.querySelector('#modalAddress').textContent = `${apartment.city} · ${apartment.address}`;
  document.querySelector('#bookingGuests').max = apartment.guests;
  document.querySelector('#bookingGuests').value = Math.min(Number(document.querySelector('#guests').value || 1), apartment.guests);
  document.querySelector('#bookingCheckIn').value = document.querySelector('#checkIn').value;
  document.querySelector('#bookingCheckOut').value = document.querySelector('#checkOut').value;
  bookingResult.classList.add('hidden');
  bookingResult.classList.remove('error');
  bookingForm.classList.remove('hidden');
  modalBackdrop.classList.remove('hidden');
}

function closeBooking() {
  modalBackdrop.classList.add('hidden');
}

searchForm.addEventListener('submit', event => {
  event.preventDefault();
  loadApartments();
});

document.querySelector('#closeModal').addEventListener('click', closeBooking);
modalBackdrop.addEventListener('click', event => {
  if (event.target === modalBackdrop) closeBooking();
});

bookingForm.addEventListener('submit', async event => {
  event.preventDefault();
  const submit = bookingForm.querySelector('button[type="submit"]');
  submit.disabled = true;
  submit.textContent = 'Создаём…';

  const payload = {
    apartment_id: Number(document.querySelector('#apartmentId').value),
    guest_name: document.querySelector('#guestName').value.trim(),
    email: document.querySelector('#email').value.trim(),
    check_in: document.querySelector('#bookingCheckIn').value,
    check_out: document.querySelector('#bookingCheckOut').value,
    guests: Number(document.querySelector('#bookingGuests').value)
  };

  try {
    const response = await fetch('/api/bookings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const data = await readJSON(response);
    bookingForm.classList.add('hidden');
    bookingResult.classList.remove('hidden', 'error');
    bookingResult.innerHTML = `
      <strong>Бронирование создано</strong><br>
      Код подтверждения: <b>${escapeHTML(data.confirmation)}</b><br>
      Запрос обработал узел: <b>${escapeHTML(data.backend_node)}</b>
    `;
    nodeBadge.textContent = `backend: ${data.backend_node}`;
    await loadApartments();
  } catch (err) {
    bookingResult.classList.remove('hidden');
    bookingResult.classList.add('error');
    bookingResult.textContent = err.message;
  } finally {
    submit.disabled = false;
    submit.textContent = 'Забронировать';
  }
});

function escapeHTML(value) {
  return String(value).replace(/[&<>'"]/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[ch]));
}

const today = new Date();
const tomorrow = new Date(today); tomorrow.setDate(today.getDate() + 1);
const after = new Date(today); after.setDate(today.getDate() + 4);
const iso = d => d.toISOString().slice(0, 10);
document.querySelector('#checkIn').min = iso(today);
document.querySelector('#checkOut').min = iso(tomorrow);
document.querySelector('#bookingCheckIn').min = iso(today);
document.querySelector('#bookingCheckOut').min = iso(tomorrow);
document.querySelector('#checkIn').value = iso(tomorrow);
document.querySelector('#checkOut').value = iso(after);

loadNode();
loadApartments();
