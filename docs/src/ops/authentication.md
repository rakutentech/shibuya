# Authentication model and multi tenancy

## Basic

Currently, Shibuya supports LDAP based authentication and no authentication. No authentication is mostly used by Shibuya developers. 

Please bear in mind, a more robust authentication is still WIP. It's not recommended to run Shibuya in a public network.

If you choose to disable authentication, that also disables multi tenancy. All the resources will be belong to a hardcoded user name `shibuya`.

## LDAP authentication

When user logs in, all the credentials will be checked against a configured LDAP server. Once it's validated, the mailing list of this user will be stored and later used as ownership source. In other words, all the resources created by the user belong to the mailing lists users are in. 

All the LDAP related configurations will be explained at this [chaper](./config.md).