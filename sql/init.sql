CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS apartments (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    city TEXT NOT NULL,
    address TEXT NOT NULL,
    description TEXT NOT NULL,
    price_night INTEGER NOT NULL CHECK (price_night > 0),
    guests INTEGER NOT NULL CHECK (guests > 0),
    rooms INTEGER NOT NULL CHECK (rooms > 0),
    rating NUMERIC(2,1) NOT NULL CHECK (rating BETWEEN 0 AND 5),
    accent TEXT NOT NULL DEFAULT 'sand'
);

CREATE TABLE IF NOT EXISTS bookings (
    id BIGSERIAL PRIMARY KEY,
    confirmation TEXT NOT NULL UNIQUE DEFAULT ('FB-' || upper(substr(md5(random()::text || clock_timestamp()::text), 1, 8))),
    apartment_id BIGINT NOT NULL REFERENCES apartments(id) ON DELETE RESTRICT,
    guest_name TEXT NOT NULL,
    email TEXT NOT NULL,
    check_in DATE NOT NULL,
    check_out DATE NOT NULL,
    guests INTEGER NOT NULL CHECK (guests > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (check_out > check_in),
    EXCLUDE USING gist (
        apartment_id WITH =,
        daterange(check_in, check_out, '[)') WITH &&
    )
);

INSERT INTO apartments (title, city, address, description, price_night, guests, rooms, rating, accent)
SELECT * FROM (VALUES
    ('Светлая студия у центра', 'Москва', 'ул. Покровка, 18', 'Тихая студия для короткой поездки: рабочее место, кухня и быстрый Wi‑Fi.', 5200, 2, 1, 4.9, 'blue'),
    ('Лофт с панорамными окнами', 'Москва', 'Ходынский б-р, 11', 'Просторный лофт рядом с метро и парком. Подходит для пары или деловой поездки.', 7600, 3, 1, 4.8, 'violet'),
    ('Квартира у набережной', 'Санкт-Петербург', 'наб. реки Фонтанки, 64', 'Двухкомнатная квартира в историческом центре, отдельная спальня и большая кухня.', 6900, 4, 2, 4.9, 'green'),
    ('Семейные апартаменты', 'Казань', 'ул. Баумана, 35', 'Две спальни, полноценная кухня и место для работы. Пешком до основных достопримечательностей.', 6100, 5, 3, 4.7, 'orange'),
    ('Минималистичная студия', 'Екатеринбург', 'ул. Малышева, 42', 'Компактная квартира с бесконтактным заселением и всем необходимым для 1–2 гостей.', 3900, 2, 1, 4.8, 'rose'),
    ('Апартаменты с видом на Волгу', 'Нижний Новгород', 'Верхне-Волжская наб., 8', 'Светлая квартира с видом, отдельной спальней и гостиной зоной.', 5800, 4, 2, 4.9, 'navy')
) AS seed(title, city, address, description, price_night, guests, rooms, rating, accent)
WHERE NOT EXISTS (SELECT 1 FROM apartments);
