import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  Button,
  Dialog,
  DialogContent,
  DialogTitle,
  Divider,
  IconButton,
  List,
  ListItem,
  ListItemSecondaryAction,
  ListItemText,
  makeStyles,
  Menu,
  MenuItem,
  TextField,
  Toolbar,
  Typography,
} from "@material-ui/core";
import { Add as AddIcon, MoreVert as MoreVertIcon } from "@material-ui/icons";
import ExpandMoreIcon from "@material-ui/icons/ExpandMore";
import clsx from "clsx";
import dayjs from "dayjs";
import React, { FC, memo, useCallback, useState } from "react";
import { useDispatch, useSelector } from "react-redux";
import { AddPipedDrawer } from "../../components/add-piped-drawer";
import { EditPipedDrawer } from "../../components/edit-piped-drawer";
import { AppState } from "../../modules";
import {
  clearRegisteredPipedInfo,
  disablePiped,
  enablePiped,
  fetchPipeds,
  Piped,
  recreatePipedKey,
  RegisteredPiped,
  selectAll,
} from "../../modules/pipeds";
import { AppDispatch } from "../../store";

const useStyles = makeStyles((theme) => ({
  item: {
    backgroundColor: theme.palette.background.paper,
  },
  disabledPipedsAccordion: {
    padding: 0,
  },
  disabledItemsSummary: {
    borderBottom: "1px solid rgba(0, 0, 0, .125)",
  },
  pipedsList: {
    flex: 1,
  },
  disabledItemsSecondaryHeader: {
    color: theme.palette.text.secondary,
    marginLeft: theme.spacing(3),
  },
  disabledItem: {
    opacity: 0.6,
  },
}));

const ITEM_HEIGHT = 48;

const usePipeds = (): [Piped[], Piped[]] => {
  const pipeds = useSelector<AppState, Piped[]>((state) =>
    selectAll(state.pipeds)
  );

  const disabled: Piped[] = [];
  const enabled: Piped[] = [];

  pipeds.forEach((piped) => {
    if (piped.disabled) {
      disabled.push(piped);
    } else {
      enabled.push(piped);
    }
  });

  return [enabled, disabled];
};

export const SettingsPipedPage: FC = memo(function SettingsPipedPage() {
  const classes = useStyles();
  const [isOpenForm, setIsOpenForm] = useState(false);
  const [actionTarget, setActionTarget] = useState<Piped | null>(null);
  const [editPipedId, setEditPipedId] = useState<string | null>(null);
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null);
  const isOpenMenu = Boolean(anchorEl);
  const dispatch = useDispatch<AppDispatch>();
  const [enabledPipeds, disabledPipeds] = usePipeds();

  const registeredPiped = useSelector<AppState, RegisteredPiped | null>(
    (state) => state.pipeds.registeredPiped
  );

  const handleMenuOpen = useCallback(
    (event: React.MouseEvent<HTMLButtonElement>, piped: Piped): void => {
      setActionTarget(piped);
      setAnchorEl(event.currentTarget);
    },
    []
  );

  const closeMenu = useCallback(() => {
    setAnchorEl(null);
    setTimeout(() => {
      setActionTarget(null);
    }, 200);
  }, []);

  const handleDisableClick = useCallback(() => {
    closeMenu();
    if (!actionTarget) {
      return;
    }

    const act = actionTarget.disabled ? enablePiped : disablePiped;

    dispatch(act({ pipedId: actionTarget.id })).then(() => {
      dispatch(fetchPipeds(true));
    });
  }, [dispatch, actionTarget, closeMenu]);

  const handleClose = useCallback(() => {
    setIsOpenForm(false);
  }, []);

  const handleClosePipedInfo = useCallback(() => {
    dispatch(clearRegisteredPipedInfo());
    dispatch(fetchPipeds(true));
  }, [dispatch]);

  const handleRecreate = useCallback(() => {
    if (actionTarget) {
      dispatch(recreatePipedKey({ pipedId: actionTarget.id }));
    }
    closeMenu();
  }, [dispatch, actionTarget, closeMenu]);

  const handleEdit = useCallback(() => {
    if (actionTarget) {
      setEditPipedId(actionTarget.id);
    }
    closeMenu();
  }, [actionTarget, closeMenu]);

  const handleEditClose = useCallback(() => {
    setEditPipedId(null);
  }, []);

  return (
    <>
      <Toolbar variant="dense">
        <Button
          color="primary"
          startIcon={<AddIcon />}
          onClick={() => setIsOpenForm(true)}
        >
          ADD
        </Button>
      </Toolbar>
      <Divider />

      <Box height="100%" overflow="auto">
        <List disablePadding className={classes.pipedsList}>
          {enabledPipeds.map((piped) => (
            <ListItem
              key={`pipe-${piped.id}`}
              divider
              dense
              className={classes.item}
            >
              <ListItemText
                primary={`${piped.name}: ${piped.version}`}
                secondary={`${piped.desc}: ${piped.id}`}
              />
              <Box>{dayjs(piped.startedAt * 1000).fromNow()}</Box>
              <ListItemSecondaryAction>
                <IconButton
                  edge="end"
                  aria-label="open menu"
                  onClick={(e) => handleMenuOpen(e, piped)}
                >
                  <MoreVertIcon />
                </IconButton>
              </ListItemSecondaryAction>
            </ListItem>
          ))}
        </List>

        <Accordion>
          <AccordionSummary
            expandIcon={<ExpandMoreIcon />}
            className={classes.disabledItemsSummary}
          >
            <Typography>Disabled pipeds</Typography>
            <Typography
              className={classes.disabledItemsSecondaryHeader}
            >{`Items: ${disabledPipeds.length}`}</Typography>
          </AccordionSummary>
          <AccordionDetails className={classes.disabledPipedsAccordion}>
            <List disablePadding className={classes.pipedsList}>
              {disabledPipeds.map((piped) => (
                <ListItem
                  key={`pipe-${piped.id}`}
                  divider
                  dense
                  className={clsx(classes.item, classes.disabledItem)}
                >
                  <ListItemText
                    primary={`${piped.name}: ${piped.version}`}
                    secondary={`${piped.desc}: ${piped.id}`}
                  />
                  {dayjs(piped.startedAt * 1000).fromNow()}
                  <ListItemSecondaryAction>
                    <IconButton
                      edge="end"
                      aria-label="open menu"
                      onClick={(e) => handleMenuOpen(e, piped)}
                    >
                      <MoreVertIcon />
                    </IconButton>
                  </ListItemSecondaryAction>
                </ListItem>
              ))}
            </List>
          </AccordionDetails>
        </Accordion>
      </Box>

      <Menu
        id="piped-menu"
        anchorEl={anchorEl}
        keepMounted
        open={isOpenMenu}
        onClose={() => closeMenu()}
        PaperProps={{
          style: {
            maxHeight: ITEM_HEIGHT * 4.5,
            width: "20ch",
          },
        }}
      >
        {actionTarget && actionTarget.disabled ? (
          <MenuItem onClick={handleDisableClick}>Enable</MenuItem>
        ) : (
          [
            <MenuItem key="piped-menu-edit" onClick={handleEdit}>
              Edit
            </MenuItem>,
            <MenuItem key="piped-menu-recreate" onClick={handleRecreate}>
              Recreate Key
            </MenuItem>,
            <MenuItem key="piped-menu-disable" onClick={handleDisableClick}>
              Disable
            </MenuItem>,
          ]
        )}
      </Menu>

      <AddPipedDrawer open={isOpenForm} onClose={handleClose} />
      <EditPipedDrawer pipedId={editPipedId} onClose={handleEditClose} />

      <Dialog open={Boolean(registeredPiped)}>
        <DialogTitle>Piped registered</DialogTitle>
        <DialogContent>
          <TextField
            label="id"
            variant="outlined"
            value={registeredPiped?.id || ""}
            fullWidth
            margin="dense"
          />
          <TextField
            label="secret key"
            variant="outlined"
            value={registeredPiped?.key || ""}
            fullWidth
            margin="dense"
          />
          <Box display="flex" justifyContent="flex-end" m={1} mt={2}>
            <Button color="primary" onClick={handleClosePipedInfo}>
              CLOSE
            </Button>
          </Box>
        </DialogContent>
      </Dialog>
    </>
  );
});
