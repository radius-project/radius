# Your first commit: Creating a forked repo

In order to limit the branches on the main repo, Radius will enforce code contributions via forked repos.

## Creating a forked repo

In order to fork a repo, go to the main radius repo page in github and select the "fork" option as shown.

<img width="800px" src="fork.png" alt="Creating a fork of the main repo">

## Configuring remote repositories

### Adding remote repositories

```
git remote add origin <url of the forked repo>
git remote add radius <url of the radius main repo>
```

### Verify new remote

```
git remote -v
```

## Workflow to follow
1. Push your local code changes to the forked repo
    ```
    git push origin <local branch>
    ```
2. Go to the main radius repo and create a pull request using the option "compare against forks" as shown

<img width="800px" src="compare.png" alt="Creating a pull request across forks">

    Make sure you select the fields (shown in red) "head repository" as your fork (<username>/radius) and "compare" as your branch with the code changes.