alter table pokemon
    drop index `ix_expire_timestamp`;

alter table pokemon
    add index `ix_expire_timestamp_verified` (`expire_timestamp`, `expire_timestamp_verified`);

ALTER TABLE pokemon_history
    ADD KEY `expire_timestamp` (`expire_timestamp`);
