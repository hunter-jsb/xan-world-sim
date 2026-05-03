-- +goose Up
-- +goose StatementBegin
-- New region kinds for the glacial-peak world (~205kya):
--   glacier: continental ice sheet covering the future cradle and Eastern Sea basin
--   agraria: the NW continental shelf exposed when the Brine receded
INSERT INTO regions (id, name, kind) VALUES
    (11, 'Cradle Ice Sheet',  'glacier'),
    (12, 'Agraria (exposed)', 'agraria');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- region_cells has ON DELETE CASCADE; this also wipes any cells of these kinds.
DELETE FROM regions WHERE id IN (11, 12);
-- +goose StatementEnd
