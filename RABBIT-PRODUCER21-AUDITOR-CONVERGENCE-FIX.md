# Rabbit Chain — correção de convergência do auditor produtor 21

O node21 foi cadastrado por 21/21 nós, produziu e a rede posteriormente
convergiu com atraso máximo de um bloco. O auditor antigo, porém, dava FAIL se
o primeiro bloco candidato não fosse confirmado por todos em 60 segundos.

Esta correção:

- usa todo o timeout global para confirmação;
- descarta um candidato que se tornou órfão e continua procurando;
- exige convergência canônica depois de REGISTER, HEARTBEAT, EXIT e retorno;
- registra cada gate no relatório final;
- não modifica nenhum arquivo Go ou regra de consenso.
