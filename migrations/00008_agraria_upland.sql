-- +goose Up
-- +goose StatementBegin
-- Split Agraria into two sub-zones: the original shelf (coast, lower) and
-- the new upland (higher, exposed earlier as sea level drops). Together
-- they give the NW more definition and a staged reveal during glacial
-- onset.
INSERT INTO regions (id, name, kind) VALUES
    (13, 'Agraria Upland', 'agraria_upland');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM regions WHERE id = 13;
-- +goose StatementEnd
