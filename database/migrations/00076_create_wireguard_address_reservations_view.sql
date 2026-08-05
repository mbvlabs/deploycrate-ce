-- +goose Up
-- +goose StatementBegin
CREATE VIEW wireguard_address_reservations AS
    SELECT host(private_address) AS private_address FROM wireguard_peers
    UNION
    SELECT host(private_address) AS private_address FROM wireguard_devices
    UNION
    SELECT allocated_address AS private_address FROM node_enrollments;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP VIEW wireguard_address_reservations;
-- +goose StatementEnd
